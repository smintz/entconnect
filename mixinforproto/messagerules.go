package mixinforproto

import (
	"fmt"
	"sort"

	"buf.build/go/protovalidate"
	pvcel "buf.build/go/protovalidate/cel"
	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// This file is VAL-08/D-10's opt-in for message-level (cross-field)
// protovalidate rules. mixinforproto.md §4.3 is normative — already
// settled upstream, not re-litigated here — on two points: message-level
// rules stay boundary-only unless a schema explicitly opts in, and the
// opt-in is Create-only (OnUpdateWithFetch, a hypothetical Update-side
// opt-in that would fetch the pre-mutation row to reconstruct a complete
// entity, is deliberately NOT declared: hidden query cost and unclear
// concurrent-write semantics, named in mixinforproto.md §4.3 as a
// deliberately-excluded-from-v1 future option). D-10 resolves the ONE
// question §4.3 leaves open: what happens when a message-level rule
// references a field mixinforproto does not have (excluded, overridden,
// or underivable)? Constructing an M from mutation fields alone would
// leave that field at its proto3 zero, and the message-level rule would
// return a verdict — pass or fail — computed from data that was never
// real (D-03's phantom-violation class, at message scope). D-10 makes
// that a schema-load panic instead, naming every offending (rule, field)
// pair in one collected pass, exactly as errors.go's failure/
// derivationError machinery already does for every other schema-load
// failure in this package.
//
// checkMessageRuleReferences implements D-10's walk. It is called from
// buildHookState (hooks.go) — the only place this package evaluates
// message-level rules at all — never from derive.go: Fields()/
// Annotations() need no knowledge of message-level rules (they never
// contribute an ent field), so the check belongs exactly where the
// consuming code lives, matching hooks.go's own existing failures/
// newDerivationError collected-failure pattern rather than adding a
// second, ad hoc panic mechanism.

// MessageRuleTrigger names when WithMessageRules enforces a message's
// message-level (cross-field) protovalidate rules at the storage layer.
// Its only declared value is OnCreate — mixinforproto.md §4.3 names
// OnUpdateWithFetch as a deliberately-excluded-from-v1 future option
// (hidden query cost, unclear concurrent-write semantics), and Phase 1's
// own precedent is that a symbol that lies (a declared-but-inert
// OnUpdateWithFetch) is worse than one that is simply absent — see
// option.go's WithMessageRules doc comment.
type MessageRuleTrigger int

const (
	// OnCreate enforces a schema's message-level rules on Create only.
	// mixinforproto never fetches the pre-mutation row to reconstruct a
	// complete entity on Update — see this type's own doc comment.
	OnCreate MessageRuleTrigger = iota
)

// checkMessageRuleReferences resolves md's message-level rules
// (protovalidate.ResolveMessageRules) and, for every custom
// (buf.validate.message).cel rule, compiles it once and statically
// enumerates every top-level this.<field> select it makes (see
// fieldSelectsOnThis's doc comment for the resolved Open Question 3
// mechanism). Every referenced field this package cannot reconstruct at
// mutation time — excluded via Exclude, replaced via Override, or
// underivable — is collected into failures, naming the message, the
// rule id, and the field, in the same failure/newDerivationError shape
// every other schema-load failure in this package uses. When failures is
// empty, refs is the complete, deduplicated, deterministically ordered
// set of every field descriptor md's message-level rules reference — the
// set buildHookState must guarantee gets reverse-converted into the
// reconstructed dynamicpb message regardless of whether any of those
// fields also carries a field-level rule of its own (D-03's
// phantom-violation protection extended to message scope: a field a
// message rule reads but which carries no rule of its OWN would
// otherwise never be reverse-converted at all, leaving it at its proto3
// zero even though real mutation data exists for it).
//
// md with no message-level rules at all (Test 7's MessageRuleNone) is a
// legal no-op: both return values are nil, and buildHookState adds no
// evaluation work for it.
func checkMessageRuleReferences(msgName string, md protoreflect.MessageDescriptor, o *options) (refs []protoreflect.FieldDescriptor, failures []failure) {
	msgRules, err := protovalidate.ResolveMessageRules(md)
	if err != nil {
		return nil, []failure{{
			message:     msgName,
			field:       "",
			fieldIndex:  fieldIndexMessageScoped,
			rule:        "WithMessageRules",
			description: fmt.Sprintf("resolving protovalidate message rules: %v", err),
			remedy:      "this is a protovalidate/descriptor-resolution failure, not a contract error",
		}}
	}
	if msgRules == nil || len(msgRules.GetCel()) == 0 {
		return nil, nil
	}

	env, eerr := messageRuleCELEnv(md)
	if eerr != nil {
		return nil, []failure{{
			message:     msgName,
			field:       "",
			fieldIndex:  fieldIndexMessageScoped,
			rule:        "WithMessageRules",
			description: fmt.Sprintf("building CEL environment for message-level rules: %v", eerr),
			remedy:      "this is an infrastructure failure, not a contract error",
		}}
	}

	seenRef := map[string]bool{}
	seenFailure := map[string]bool{}
	var out []failure

	for _, r := range msgRules.GetCel() {
		ast, iss := env.Compile(r.GetExpression())
		if iss.Err() != nil {
			out = append(out, failure{
				message:     msgName,
				field:       "",
				fieldIndex:  fieldIndexMessageScoped,
				rule:        "WithMessageRules",
				description: fmt.Sprintf("compiling message rule %q (%q): %v", r.GetId(), r.GetExpression(), iss.Err()),
				remedy:      "fix the CEL expression in the contract's (buf.validate.message).cel rule",
			})
			continue
		}

		for _, name := range fieldSelectsOnThis(ast) {
			// Byte-exact lookup against protoreflect.Name — never the
			// JSON name, never case-insensitively — mirroring
			// validateOptionNames' matching discipline (derive.go). A
			// successfully compiled expression against a message-typed
			// "this" env can only select a real declared field: CEL's
			// own type checker rejects a select naming a field the
			// message descriptor does not declare, so this lookup
			// failing here is defensive, not a reachable path.
			fd := md.Fields().ByName(protoreflect.Name(name))
			if fd == nil {
				continue
			}

			reason, unavailable := messageRuleFieldUnavailable(msgName, fd, o)
			if unavailable {
				key := r.GetId() + "\x00" + name
				if !seenFailure[key] {
					seenFailure[key] = true
					out = append(out, failure{
						message:     msgName,
						field:       name,
						fieldIndex:  int(fd.Index()),
						rule:        "WithMessageRules",
						description: fmt.Sprintf("message rule %q references field %q, which %s", r.GetId(), name, reason),
						remedy:      fmt.Sprintf("stop excluding/overriding %q, or do not opt into WithMessageRules(OnCreate) for this message", name),
					})
				}
				continue
			}

			if !seenRef[name] {
				seenRef[name] = true
				refs = append(refs, fd)
			}
		}
	}

	sort.Slice(refs, func(i, j int) bool { return refs[i].Index() < refs[j].Index() })

	return refs, out
}

// messageRuleFieldUnavailable reports whether fd — a field a message-
// level rule references via this.<field> — is one mixinforproto cannot
// reconstruct at mutation time, and if so, why: excluded via Exclude,
// replaced via Override (which suppresses validation relay for that
// field entirely, option.go's own documented semantics), or underivable
// (mapField produces no ent field at all, or the field's derivation
// class is not one buildHookState's hookFieldClass routes through the
// storage-layer hook yet). The returned reason is a fragment completing
// "message rule %q references field %q, which %s".
func messageRuleFieldUnavailable(msgName string, fd protoreflect.FieldDescriptor, o *options) (reason string, unavailable bool) {
	name := string(fd.Name())

	if o.isExcluded(name) {
		return "is excluded via Exclude(...)", true
	}
	if o.isOverridden(name) {
		return "is replaced via Override(...), which suppresses validation relay for it entirely", true
	}

	f, err := mapField(msgName, fd, o)
	if err != nil {
		return fmt.Sprintf("fails to derive (%v)", err), true
	}
	if f == nil {
		return "derives no ent field at all (its derivation kind produces none)", true
	}
	if hookFieldClass(fd) == "" {
		return "derives an ent field, but its derivation kind is not yet routed through the storage-layer hook", true
	}

	return "", false
}

// messageRuleCELEnv builds the cel.Env this file compiles md's
// message-level rules against, matching protovalidate's own message-
// level environment construction exactly (buf.build/go/protovalidate@
// v1.2.0/builder.go's processMessageExpressions, verified this session):
// pvcel.NewLibrary() supplies protovalidate's custom CEL function set,
// cel.Types(dynamicpb.NewMessage(md)) registers md's own type with the
// environment, and cel.Variable("this", cel.ObjectType(md.FullName()))
// types "this" as the message itself rather than a single field's type
// (compileCELRule's field-scoped construction, hooks.go). Building the
// env this way is what makes "this.<field> fails to compile" mean
// exactly what protovalidate's own message-level evaluator would also
// reject — the same reasoning compileCELRule's doc comment gives for the
// field-scoped case.
func messageRuleCELEnv(md protoreflect.MessageDescriptor) (*cel.Env, error) {
	return cel.NewEnv(
		cel.Lib(pvcel.NewLibrary()),
		cel.Types(dynamicpb.NewMessage(md)),
		cel.Variable("this", cel.ObjectType(string(md.FullName()))),
	)
}

// fieldSelectsOnThis statically enumerates every top-level this.<field>
// select ast makes, resolving 03-RESEARCH.md Open Question 3: whether
// cel.Ast.NativeRep()'s common/ast.AST exposes a mechanism that
// distinguishes a genuine field select from an unrelated identifier or
// function-call reference.
//
// Resolved mechanism, prototyped against exactly the shapes
// MessageRuleOk/MessageRuleLookalike exercise before this function was
// written: common/ast.AST.ReferenceMap() (the candidate 03-RESEARCH.md
// named) only records identifier/constant/function references — it has
// no entry at all for a select expression's field target, so it cannot
// make this distinction. The AST's navigable expression tree
// (common/ast.NavigateAST + MatchDescendants(KindMatcher(SelectKind)))
// does: walking every SelectKind node and checking whether its Operand
// is exactly the identifier "this" finds every direct field reference
// with no false positives, because a function call, a comprehension-
// local variable, and a string literal each produce a different node
// kind (CallKind, IdentKind, a literal constant) that never matches
// SelectKind at all — they are structurally excluded from the walk, not
// merely absent from a name match. Verified directly: compiling
// `[1,2,3].exists(lo, lo == 1) && size('this.lo is not a real
// reference') >= 0 && this.hi >= 0` against a real cel.Env returns
// exactly {"hi"} — the comprehension-bound "lo" and the string literal
// naming "this.lo" are both correctly excluded, and the real `this.hi`
// select is correctly found.
//
// A nested select (this.a.b) resolves only "a": "b" is selected off "a"
// (the field's own value), not off "this" directly, so it is out of
// scope for this one-level-deep walk — matching newFieldViolation's own
// one-field-deep FieldPath shape (hooks.go), which this file's callers
// never need to look past for a top-level message-level rule.
func fieldSelectsOnThis(ast *cel.Ast) []string {
	native := ast.NativeRep()
	nav := celast.NavigateAST(native)
	selects := celast.MatchDescendants(nav, celast.KindMatcher(celast.SelectKind))

	seen := map[string]bool{}
	var out []string
	for _, sel := range selects {
		se := sel.AsSelect()
		operand := se.Operand()
		if operand.Kind() == celast.IdentKind && operand.AsIdent() == "this" {
			name := se.FieldName()
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}
