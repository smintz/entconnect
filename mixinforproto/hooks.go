package mixinforproto

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"entgo.io/ent"
	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"
	pvcel "buf.build/go/protovalidate/cel"
)

// This file is mixinforproto's first ent.Mixin.Hooks() implementation
// (VAL-04). D-07 specifies a hybrid evaluator: protovalidate's own
// evaluator for every standard rule on an in-scope field — translated and
// residual alike (D-02/D-08) — plus a local cel.Env for custom
// (buf.validate.field).cel rules, both compiled once at schema load. Plan
// 03-01 built only the residual-CEL half as this phase's thin tracer
// slice; this plan (03-03) completes the hybrid: buildHookState now also
// precompiles a protovalidate.Validator per message, and evaluate() below
// routes every in-scope, rule-bearing field through it via a
// D-03/D-06/VAL-08-shaped Filter, alongside the unchanged local-CEL path.
//
// Compilation discipline (VAL-04 "compiled once at schema load", T-03-04
// DoS mitigation): every cel.Env/cel.Program AND the one
// protovalidate.Validator this file builds are built inside
// buildHookState, called from protoMixin[M].Hooks() (mixin.go) — NEVER
// inside the ent.Hook closure hook() returns. The returned closure only
// ever evaluates already-compiled/constructed state.
//
// D-06's operation-dependent scope (Create: all derived fields; Update:
// changed-only) is implemented as a SINGLE m.Fields() read with no
// ent.Op() branch — see inScopeFieldNames's doc comment for the recorded
// empirical evidence this collapse relies on (03-01-SUMMARY.md,
// TestUnsetDefaultFieldAppearsInMutationFieldsOnCreate).
//
// D-08's hybrid split is drawn by rule KIND, not by field: every in-scope,
// class-resolvable field carrying ANY protovalidate rule (standard or
// custom CEL) is evaluated by protovalidate's own evaluator; a field
// additionally carrying a custom CEL rule is ALSO evaluated by the local
// cel.Env for that rule. 03-RESEARCH.md Pitfall 1 established that
// protovalidate's Filter mechanism cannot split a single field's
// structural rules from its custom-CEL rule — there is no sub-field
// granularity — so a mixed field's CEL rule really is evaluated twice, by
// two independently-compiled engines. violation.go's newValidationError is
// where that accepted cost is resolved: both evaluators' violations funnel
// through it, and it deduplicates by (RuleId, FieldPath) before returning
// the merged *protovalidate.ValidationError (Pitfall 1's resolution (a)).

// celCompileCount is a package-level test seam (03-01-PLAN.md acceptance
// criteria): incremented once per compiled cel.Program, so a test can
// assert compilation happens exactly at Hooks() construction time and
// never inside the returned hook closure. It is not part of this
// package's stable public API — it exists solely so the differential
// test harness under internal/difftest can observe compilation timing
// empirically, from outside this package, without any generated ent code
// needing to expose anything itself.
var celCompileCount atomic.Int64

// CELCompileCount returns the number of residual CEL programs compiled
// by this process so far. See celCompileCount's doc comment: this is a
// test-only instrument, not a guarantee about this package's stable
// public API.
func CELCompileCount() int64 {
	return celCompileCount.Load()
}

// standardValidatorBuildCount is 03-03's sibling test seam to
// celCompileCount above: incremented once per protovalidate.Validator
// this file constructs, so a test can assert the standard-rule evaluator
// half of D-07's hybrid is also built exactly once per mixin construction
// (Task 1's Test 6), never inside the returned hook closure.
var standardValidatorBuildCount atomic.Int64

// StandardValidatorBuildCount returns the number of standard-rule
// protovalidate.Validator instances built by this process so far. Test-only,
// mirroring CELCompileCount's doc comment.
func StandardValidatorBuildCount() int64 {
	return standardValidatorBuildCount.Load()
}

// celProgram is one compiled, schema-load-time-ready residual CEL rule
// for a single field.
type celProgram struct {
	ruleID     string
	message    string
	expression string
	prg        cel.Program
}

// fieldEvaluator is the schema-load-compiled state for one field carrying
// at least one protovalidate rule (standard, custom CEL, or both) that
// this package can reverse-bind. programs is non-empty only when fd
// carries a (buf.validate.field).cel rule; a field with a standard rule
// only still gets an entry here (with a nil/empty programs slice) so
// evaluate() knows to route it through the standard-rule evaluator's
// Filter scope.
type fieldEvaluator struct {
	fd       protoreflect.FieldDescriptor
	class    string
	programs []celProgram
	// ignore is fd's resolved (buf.validate.field).ignore mode
	// (validate.Ignore_IGNORE_UNSPECIFIED when the field carries none),
	// resolved ONCE here at schema load — VAL-04's "compiled once at
	// schema load" wording extends to this decision too, so evaluate()
	// must never call protovalidate.ResolveFieldRules at mutation time
	// (CR-02 gap closure, 03-06-PLAN.md).
	ignore validate.Ignore
}

// hookState is the schema-load-time compiled state behind one
// MixinForProto[M]'s Hooks() entry: D-07's complete hybrid for one
// message type. md is retained for two mutation-time uses — building the
// dynamicpb reconstruction evaluate() hands to the standard-rule
// evaluator, and resolving a returned violation's field number back to
// its descriptor index for violation.go's deterministic sort.
type hookState struct {
	msgName    string
	md         protoreflect.MessageDescriptor
	validator  protovalidate.Validator
	evaluators []fieldEvaluator
	// messageRulesOnCreate records whether WithMessageRules(OnCreate) was
	// passed (option.go/messagerules.go, VAL-08/D-10). It is the ONE
	// boolean evaluate()'s Filter checks to decide whether the message
	// descriptor itself is in scope for protovalidate's own evaluator —
	// no second evaluation path, per messagerules.go's own doc comment.
	messageRulesOnCreate bool
}

// buildHookState walks md's fields at schema-load time, resolving each
// field's protovalidate rules via the same ResolveFieldRules path Tier 1
// already uses (D-03/fieldmap.go's resolvedFieldRules). A field with NO
// protovalidate rules at all is never added to evaluators, so a message
// with no constraints anywhere pays no mutation-time cost and Hooks()
// returns no hook at all for it (mixin.go's len(hs.evaluators) == 0
// check). A field WITH a rule but an unbindable derivation class (D-09;
// hookFieldClass returns "") is skipped the same way — that gap is
// mixinforproto's own, recorded elsewhere as boundary-only provenance
// (derive.go's recordBoundaryOnly, 03-02), not panicked on here.
//
// Every field that survives both checks gets an entry in evaluators
// (D-02: standard rules and residual CEL alike are in scope), and — only
// if it carries at least one (buf.validate.field).cel rule — a compiled
// cel.Program per rule. An uncompilable CEL expression becomes a
// collected failure fed through newDerivationError (D-09's
// uncompilable-CEL panic path) — never a second, ad hoc panic mechanism;
// every offender across every field is reported in one pass, matching
// Phase 1 D-09's discipline.
//
// If at least one field needs standard-rule enforcement, this function
// also precompiles the ONE protovalidate.Validator this hook's standard
// half uses for the lifetime of the mixin — protovalidate.New with
// WithMessages(the message's own zero-value dynamicpb instance, so no
// caller-supplied M instance is required here) and WithDisableLazy(),
// which VAL-04's literal "compiled once at schema load" wording requires
// for this half exactly as it already does for the residual-CEL half
// (03-RESEARCH.md Pattern 1). A construction failure is a collected
// failure fed through the same newDerivationError path, never a bare
// panic.
//
// opts carries the SAME Option values the mixin's Fields()/Annotations()
// were constructed with (mixin.go's protoMixin[M].opts) — needed here so
// this function can see whether WithMessageRules(OnCreate) was passed
// (VAL-08/D-10, messagerules.go). When it was, checkMessageRuleReferences
// runs BEFORE the per-field loop below: its own failures join this
// function's failures slice, and — when it finds none — its returned
// field-reference set seeds extraFields, guaranteeing every field a
// message-level rule reads gets an evaluators entry (and therefore gets
// reverse-converted into the reconstructed dynamicpb message) even when
// that field carries no protovalidate rule of its own (D-03's phantom-
// violation protection extended to message scope — see
// checkMessageRuleReferences' doc comment).
func buildHookState(md protoreflect.MessageDescriptor, opts ...Option) (*hookState, error) {
	msgName := string(md.FullName())
	o := applyOptions(opts)
	hs := &hookState{msgName: msgName, md: md, messageRulesOnCreate: o.messageRulesEnabled()}
	var failures []failure

	extraFields := map[protoreflect.FieldNumber]bool{}
	if hs.messageRulesOnCreate {
		refs, msgFailures := checkMessageRuleReferences(msgName, md, o)
		failures = append(failures, msgFailures...)
		if len(msgFailures) == 0 {
			for _, fd := range refs {
				extraFields[fd.Number()] = true
			}
		}
	}

	fds := md.Fields()
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		name := string(fd.Name())

		rules, err := protovalidate.ResolveFieldRules(fd)
		if err != nil {
			failures = append(failures, failure{
				message:     msgName,
				field:       name,
				fieldIndex:  int(fd.Index()),
				rule:        "hook",
				description: fmt.Sprintf("resolving protovalidate field rules for hook compilation: %v", err),
				remedy:      "this is a protovalidate/descriptor-resolution failure, not a contract error — check the field's proto options",
			})
			continue
		}
		isExtra := extraFields[fd.Number()]
		if rules == nil && !isExtra {
			// No protovalidate rule of any kind on this field, and no
			// message-level rule reads it either — nothing for any half
			// of the hybrid to enforce.
			continue
		}

		class := hookFieldClass(fd)
		if class == "" {
			// D-09: unbindable field kind, recorded as still-boundary-only
			// elsewhere by simply not compiling it here — see doc comment
			// above. checkMessageRuleReferences already rejected this
			// class as a message-rule reference (messageRuleFieldUnavailable),
			// so isExtra is never true here when messageRulesOnCreate
			// succeeded — this branch is field-level-rule-only territory.
			continue
		}

		// CR-02 gap closure (03-06-PLAN.md): resolve fd's ignore mode
		// ONCE here, at schema load, exactly the way every other
		// per-field decision in this loop is resolved once. When it is
		// IGNORE_ALWAYS, the local cel.Env compilation below is skipped
		// entirely — programs stays nil — so evaluate()'s residual-CEL
		// half never has a program to run for this field, matching what
		// protovalidate's own boundary evaluator already does for the
		// standard half (buf.build/go/protovalidate's own Ignore
		// handling, verified against validate.proto's own doc comment
		// on the Ignore enum). IGNORE_IF_ZERO_VALUE is NOT handled here
		// — it is a mutation-time (value-dependent) decision, gated
		// inside evaluate()'s celFields loop by isZeroForKind (Task 2),
		// not a compile-time one.
		ignoreMode := rules.GetIgnore()

		var programs []celProgram
		if ignoreMode != validate.Ignore_IGNORE_ALWAYS {
			for _, r := range rules.GetCel() {
				prg, cerr := compileCELRule(fd, r)
				if cerr != nil {
					failures = append(failures, failure{
						message:     msgName,
						field:       name,
						fieldIndex:  int(fd.Index()),
						rule:        "hook",
						description: cerr.Error(),
						remedy:      "fix the CEL expression in the contract's (buf.validate.field).cel rule",
					})
					continue
				}
				celCompileCount.Add(1)
				programs = append(programs, celProgram{
					ruleID:     r.GetId(),
					message:    r.GetMessage(),
					expression: r.GetExpression(),
					prg:        prg,
				})
			}
		}

		// D-02: this field is in scope for the standard-rule evaluator
		// regardless of whether it has any CEL programs above — a field
		// with only a standard rule (e.g. string.max_len) still needs an
		// entry so evaluate() includes it in the Filter scope.
		//
		// This entry is kept EVEN for an IGNORE_ALWAYS field — dropping
		// it would be wrong in two ways: (a) protovalidate's own
		// evaluator already honors ignore for the standard-rule half, so
		// leaving the field in the standardFields Filter scope produces
		// the boundary-identical answer (zero violations) with no
		// reimplementation of ignore semantics for structural rules
		// needed here; (b) a field a message-level rule references via
		// extraFields must still be reverse-converted into the
		// reconstructed dyn message, or D-03's phantom-violation
		// protection breaks at message scope — the CR-01 class.
		hs.evaluators = append(hs.evaluators, fieldEvaluator{
			fd:       fd,
			class:    class,
			programs: programs,
			ignore:   ignoreMode,
		})
	}

	if derr := newDerivationError(msgName, failures); derr != nil {
		return nil, derr
	}

	if len(hs.evaluators) > 0 {
		exampleMsg := dynamicpb.NewMessage(md)
		v, verr := protovalidate.New(
			protovalidate.WithMessages(exampleMsg),
			protovalidate.WithDisableLazy(),
		)
		if verr != nil {
			return nil, newDerivationError(msgName, []failure{{
				message:     msgName,
				field:       "",
				fieldIndex:  fieldIndexMessageScoped,
				rule:        "hook",
				description: fmt.Sprintf("precompiling the standard-rule protovalidate evaluator: %v", verr),
				remedy:      "check that this message's protovalidate rules compile cleanly (e.g. via a standalone protovalidate.Validate call)",
			}})
		}
		standardValidatorBuildCount.Add(1)
		hs.validator = v
	}

	return hs, nil
}

// hookFieldClass returns the SourceField.Kind-shaped derivation-class
// string reverse.go's reverseValue expects for fd, or "" if reverse.go
// has no conversion for fd's class yet (D-09's unbindable-kind case).
// Reuses fieldmap.go's classify — the identical classification Tier 1's
// forward table already applies to this same field — rather than
// re-deriving field shape a second, possibly-divergent way.
//
// Scoped to classScalar/classOptionalScalar as of this plan: every field
// this plan's corpus fixtures (MixedFieldRules and the pre-existing
// scalar-kind constraint corpus) exercise is one of these two classes.
// Extending this to enum/wkt/scalarMap/asJSON — all of which reverse.go
// can already reverse-convert as of 03-02 — is deliberately left to a
// later plan with its own dedicated corpus/tests (03-05's PIPE-06
// differential sweep is the natural place this gap would first become
// visible): those field classes remain reported as boundary-only via
// derive.go's recordBoundaryOnly (03-02) until then, exactly as they were
// before this plan.
func hookFieldClass(fd protoreflect.FieldDescriptor) string {
	switch classify(fd) {
	case classScalar:
		return "scalar"
	case classOptionalScalar:
		return "optionalScalar"
	default:
		return ""
	}
}

// compileCELRule compiles r's expression into a cel.Program against a
// cel.Env built the same way protovalidate's own evaluator builds its
// per-field environment (buf.build/go/protovalidate@v1.2.0/builder.go,
// verified in 03-RESEARCH.md): pvcel.NewLibrary() supplies protovalidate's
// exact custom CEL function set, pvcel.RequiredEnvOptions(fd) supplies
// any message-type declarations fd's kind needs, and
// pvcel.ProtoFieldToType(fd, false, false) supplies the exact "this"
// variable type protovalidate's own field evaluator uses for a
// non-generic, non-nested rule. Building the env this way — rather than
// hand-assembling a smaller one — is what makes the local env's verdict
// provably comparable to protovalidate's own (D-07 consequence 1;
// PIPE-06 is the mechanism that keeps this safe in practice).
func compileCELRule(fd protoreflect.FieldDescriptor, r *validate.Rule) (cel.Program, error) {
	celType := pvcel.ProtoFieldToType(fd, false, false)
	opts := append([]cel.EnvOption{cel.Lib(pvcel.NewLibrary())}, pvcel.RequiredEnvOptions(fd)...)
	opts = append(opts, cel.Variable("this", celType))

	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf(
			"mixinforproto: %s.%s: building CEL environment for rule %q: %w",
			fd.ContainingMessage().FullName(), fd.Name(), r.GetId(), err,
		)
	}
	ast, iss := env.Compile(r.GetExpression())
	if iss.Err() != nil {
		return nil, fmt.Errorf(
			"mixinforproto: %s.%s: compiling CEL rule %q (%q): %w",
			fd.ContainingMessage().FullName(), fd.Name(), r.GetId(), r.GetExpression(), iss.Err(),
		)
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf(
			"mixinforproto: %s.%s: building CEL program for rule %q: %w",
			fd.ContainingMessage().FullName(), fd.Name(), r.GetId(), err,
		)
	}
	return prg, nil
}

// hook returns the ent.Hook this hookState compiled at schema load. Only
// ever called once, from protoMixin[M].Hooks() (mixin.go); the returned
// closure never compiles or constructs anything — every cel.Program and
// the one protovalidate.Validator it evaluates were already built by
// buildHookState above.
func (hs *hookState) hook() ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			violations, err := hs.evaluate(m)
			if err != nil {
				// D-12: a reverse-conversion failure (or a genuine
				// evaluator failure — a CompilationError/RuntimeError
				// from protovalidate's own evaluator, or a CEL evaluation
				// error from the local env) is a data-integrity/
				// infrastructure fault, not a protovalidate violation —
				// the mutation fails closed with a plain error, never
				// wrapped as *protovalidate.ValidationError and never
				// assigned a synthesized RuleId.
				return nil, err
			}
			if len(violations) > 0 {
				// D-07 consequence 3: the ONE call site in this package
				// that builds a *protovalidate.ValidationError.
				return nil, newValidationError(hs.md, violations)
			}
			return next.Mutate(ctx, m)
		})
	}
}

// inScopeFieldNames returns the set of proto field names hook-time
// evaluation must cover for mutation m, implementing D-06's
// operation-dependent scope as a SINGLE m.Fields() read with no
// ent.Op() branch.
//
// This collapse is not a simplifying assumption — it is the mechanism
// 03-01's TestUnsetDefaultFieldAppearsInMutationFieldsOnCreate
// (mixinforproto/internal/difftest/tracer_test.go) proved empirically,
// against a real generated ent.Client, per this task's own
// <precondition>: ent's generated defaults() calls a Default()-bearing
// field's setter — materializing it into the mutation as "set" — BEFORE
// any hook, mixin or schema, ever runs on Create (create.tmpl). Every
// derived non-optional field mixinforproto produces carries
// Default(zero) (Phase 1 D-26), so by the time this hook runs on Create,
// m.Fields() already contains every derived field the caller left
// unset, with no separate "enumerate every SourceField" code path
// needed. On Update, defaults() only consults UpdateDefault-tagged
// fields (update.tmpl) — mixinforproto's derived fields never carry
// UpdateDefault — so m.Fields() there is exactly, and only, the fields a
// FieldMask-gated Set* call actually touched (D-03's phantom-violation
// protection; Pitfall 4's "no ent-side safety net on Update").
// mutation.Fields() therefore differs correctly in content between
// Create and Update because of ENT'S OWN defaults()/UpdateDefault split
// — not because this function branches on m.Op().
func inScopeFieldNames(m ent.Mutation) map[string]struct{} {
	fields := m.Fields()
	out := make(map[string]struct{}, len(fields))
	for _, name := range fields {
		out[name] = struct{}{}
	}
	return out
}

// evaluate runs D-07's complete hybrid against m's in-scope field values:
// every in-scope, rule-bearing field is reverse-converted once and handed
// to BOTH the standard-rule evaluator (protovalidate's own, scoped by a
// Filter over exactly this field set — D-03/D-08) and, if it carries a
// custom CEL rule, the local cel.Env (D-08's residual half). The two
// halves' raw *validate.Violation protos are simply concatenated here;
// violation.go's newValidationError is where (RuleId, FieldPath)
// deduplication and deterministic ordering happen (Pitfall 1's resolution
// (a) — see this file's own doc comment).
func (hs *hookState) evaluate(m ent.Mutation) ([]*validate.Violation, error) {
	if len(hs.evaluators) == 0 {
		return nil, nil
	}

	inScope := inScopeFieldNames(m)

	dyn := dynamicpb.NewMessage(hs.md)
	values := make(map[protoreflect.FieldNumber]protoreflect.Value, len(hs.evaluators))
	standardFields := make(map[protoreflect.FieldNumber]struct{}, len(hs.evaluators))
	var celFields []fieldEvaluator

	for _, fe := range hs.evaluators {
		name := string(fe.fd.Name())
		if _, ok := inScope[name]; !ok {
			continue
		}
		raw, ok := m.Field(name)
		if !ok {
			continue
		}

		val, err := reverseValue(fe.fd, fe.class, raw)
		if err != nil {
			return nil, err
		}
		if val.IsValid() {
			dyn.Set(fe.fd, val)
			values[fe.fd.Number()] = val
		}
		standardFields[fe.fd.Number()] = struct{}{}
		if len(fe.programs) > 0 {
			celFields = append(celFields, fe)
		}
	}

	if len(standardFields) == 0 {
		// D-06/Test 7: nothing in this mutation is in scope for either
		// evaluator — return without ever calling hs.validator.Validate
		// or a single cel.Program.
		return nil, nil
	}

	var violations []*validate.Violation

	// D-02/D-03/D-08: every standard rule on an in-scope field —
	// translated and residual alike, and (per Pitfall 1's resolution) a
	// mixed field's structural half too — is evaluated by protovalidate's
	// OWN evaluator, restricted to exactly the in-scope field set by the
	// Filter below. VAL-08/D-10: message-level (cross-field) rules stay
	// boundary-only by default — the filter refuses the message
	// descriptor itself, independently of the per-field checks
	// (03-RESEARCH.md Pattern 2) — UNLESS hs.messageRulesOnCreate was set
	// (WithMessageRules(OnCreate), option.go) AND this mutation is a
	// Create. This is the ONE boolean messagerules.go's own doc comment
	// promises: no second evaluation path, no separate compiled-program
	// mechanism for message-level rules — hs.validator already compiled
	// them (protovalidate's own builder always processes a message's
	// message-level CEL rules when constructing its Validator), this
	// Filter is the only thing standing between "compiled" and "actually
	// evaluated".
	messageRulesInScope := hs.messageRulesOnCreate && m.Op() == ent.OpCreate
	scope := protovalidate.FilterFunc(func(msg protoreflect.Message, d protoreflect.Descriptor) bool {
		if d == msg.Descriptor() {
			return messageRulesInScope
		}
		fd, ok := d.(protoreflect.FieldDescriptor)
		if !ok {
			// Oneof descriptors: not this phase's concern (03-RESEARCH.md
			// Pattern 2's own scope note).
			return false
		}
		_, ok = standardFields[fd.Number()]
		return ok
	})
	if verr := hs.validator.Validate(dyn, protovalidate.WithFilter(scope)); verr != nil {
		var ve *protovalidate.ValidationError
		if !errors.As(verr, &ve) {
			// A genuine evaluator failure (CompilationError/RuntimeError),
			// not a rule-violation verdict — D-12's "infrastructure
			// fault, not a violation" posture applies here too.
			return nil, fmt.Errorf(
				"mixinforproto: %s: evaluating standard protovalidate rules: %w",
				hs.msgName, verr,
			)
		}
		for _, v := range ve.Violations {
			violations = append(violations, v.Proto)
		}
	}

	// D-07/D-08's residual half: a field's custom
	// (buf.validate.field).cel rule(s), evaluated by the local cel.Env
	// regardless of whether the standard-rule evaluator above already
	// evaluated the same field (a mixed field's CEL rule is deliberately
	// evaluated by both engines — Pitfall 1's accepted cost, resolved at
	// violation.go's newValidationError, not here).
	for _, fe := range celFields {
		val, ok := values[fe.fd.Number()]
		if !ok || !val.IsValid() {
			continue
		}
		this := val.Interface()

		for _, p := range fe.programs {
			out, _, evalErr := p.prg.Eval(map[string]any{"this": this})
			if evalErr != nil {
				return nil, fmt.Errorf(
					"mixinforproto: %s: evaluating CEL rule %q on field %q: %w",
					hs.msgName, p.ruleID, fe.fd.Name(), evalErr,
				)
			}
			v, verr := celResultToViolation(fe.fd, p, out.Value())
			if verr != nil {
				return nil, verr
			}
			if v != nil {
				violations = append(violations, v)
			}
		}
	}

	return violations, nil
}

// celResultToViolation interprets a CEL program's result exactly the way
// protovalidate's own evaluator does (buf.build/go/protovalidate@v1.2.0/
// program.go's compiledProgram.eval, verified in 03-RESEARCH.md): a bool
// result of true, or an empty string result, means the rule passed; a
// bool result of false produces a violation using p.message if set,
// falling back to `"<expression>" returned false` — quoting the CEL
// EXPRESSION, matching protovalidate's own fallback text exactly
// (program.go's `fmt.Sprintf("%q returned false", expr.Source.GetExpression())`)
// so a mixed field's message text is byte-identical whichever engine
// produced it (Task 2's message-text-equality requirement) — and a
// non-empty string result produces a violation using that string as the
// message. Any other result type is a genuine evaluator failure (a
// malformed contract-authored expression), not a violation and not a
// data value — surfaced as a plain error, matching D-12's "infrastructure
// fault, not a violation" posture.
func celResultToViolation(fd protoreflect.FieldDescriptor, p celProgram, result any) (*validate.Violation, error) {
	switch v := result.(type) {
	case bool:
		if v {
			return nil, nil
		}
		msg := p.message
		if msg == "" {
			msg = fmt.Sprintf("%q returned false", p.expression)
		}
		return newFieldViolation(fd, p.ruleID, msg), nil
	case string:
		if v == "" {
			return nil, nil
		}
		return newFieldViolation(fd, p.ruleID, v), nil
	default:
		return nil, fmt.Errorf(
			"mixinforproto: %s: CEL rule %q on field %q evaluated to unexpected type %T (want bool or string)",
			fd.ContainingMessage().FullName(), p.ruleID, fd.Name(), result,
		)
	}
}

// newFieldViolation builds the *validate.Violation proto naming fd and
// ruleID, in the same FieldPath shape protovalidate's own evaluator uses
// (buf.build/go/protovalidate@v1.2.0/error_utils.go's fieldPathElement,
// unexported there — this is the one-field-deep case this plan needs;
// nested paths are a later plan's concern).
func newFieldViolation(fd protoreflect.FieldDescriptor, ruleID, message string) *validate.Violation {
	return &validate.Violation{
		Field: &validate.FieldPath{
			Elements: []*validate.FieldPathElement{
				{
					FieldNumber: proto.Int32(int32(fd.Number())),
					FieldName:   proto.String(string(fd.Name())),
					FieldType:   descriptorpb.FieldDescriptorProto_Type(fd.Kind()).Enum(),
				},
			},
		},
		RuleId:  proto.String(ruleID),
		Message: proto.String(message),
	}
}
