package mixinforproto

import (
	"context"
	"fmt"
	"sync/atomic"

	"entgo.io/ent"
	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"
	pvcel "buf.build/go/protovalidate/cel"
)

// This file is mixinforproto's first ent.Mixin.Hooks() implementation
// (VAL-04). D-07 specifies a hybrid evaluator: protovalidate's own
// evaluator for standard rules, plus a local cel.Env for residual custom
// CEL — but this plan (03-01) is the phase's thin tracer slice and
// compiles ONLY the residual half (D-08's routing line: the local env
// handles exclusively (buf.validate.field).cel expressions). The
// standard-rule half — protovalidate's own evaluator over every in-scope
// field, translated and residual alike (D-02) — is a later plan in this
// phase; hookState's shape below is deliberately built so that half can
// be added alongside this one without changing the mutation-time
// contract (one hookState, one hook() closure, one evaluate() call).
//
// Compilation discipline (VAL-04 "compiled once at schema load", T-03-04
// DoS mitigation): every cel.Env/cel.Program this file builds is built
// inside buildHookState, called from protoMixin[M].Hooks() (mixin.go) —
// NEVER inside the ent.Hook closure hook() returns. The returned closure
// only ever evaluates already-compiled programs.

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

// celProgram is one compiled, schema-load-time-ready residual CEL rule
// for a single field.
type celProgram struct {
	ruleID  string
	message string
	prg     cel.Program
}

// fieldEvaluator is the schema-load-compiled state for one field's
// residual CEL rule set.
type fieldEvaluator struct {
	fd       protoreflect.FieldDescriptor
	class    string
	programs []celProgram
}

// hookState is the schema-load-time compiled state behind one
// MixinForProto[M]'s Hooks() entry.
type hookState struct {
	msgName    string
	evaluators []fieldEvaluator
}

// buildHookState walks md's fields at schema-load time, resolving each
// field's protovalidate rules via the same ResolveFieldRules path Tier 1
// already uses (D-03/fieldmap.go's resolvedFieldRules), and compiling one
// cel.Program per (buf.validate.field).cel rule it finds. A field with no
// CEL rules is never added to evaluators, so a message with no residual
// CEL rules pays no mutation-time cost and Hooks() returns no hook at
// all for it.
//
// An uncompilable CEL expression becomes a collected failure fed through
// newDerivationError (D-09's uncompilable-CEL panic path) — never a
// second, ad hoc panic mechanism; every offender is reported in one pass,
// matching Phase 1 D-09's discipline.
//
// An unbindable field kind (a derivation class reverse.go cannot convert
// yet) is silently skipped rather than panicking (D-09): that gap is
// mixinforproto's own, not the contract's, and panicking would brick a
// schema that loaded fine under Phases 1-2. The field's residual CEL rule
// simply stays uncompiled and therefore boundary-only, exactly as it was
// before this plan — a later plan records this explicitly as provenance
// (D-09's "record as still-unenforced/boundary-only").
func buildHookState(md protoreflect.MessageDescriptor) (*hookState, error) {
	msgName := string(md.FullName())
	hs := &hookState{msgName: msgName}
	var failures []failure

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
		if rules == nil {
			continue
		}

		celRules := rules.GetCel()
		if len(celRules) == 0 {
			continue
		}

		class := hookFieldClass(fd)
		if class == "" {
			// D-09: unbindable field kind, recorded as still-boundary-only
			// by simply not compiling it — see doc comment above.
			continue
		}

		var programs []celProgram
		for _, r := range celRules {
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
				ruleID:  r.GetId(),
				message: r.GetMessage(),
				prg:     prg,
			})
		}
		if len(programs) == 0 {
			continue
		}

		hs.evaluators = append(hs.evaluators, fieldEvaluator{
			fd:       fd,
			class:    class,
			programs: programs,
		})
	}

	if derr := newDerivationError(msgName, failures); derr != nil {
		return nil, derr
	}
	return hs, nil
}

// hookFieldClass returns the SourceField.Kind-shaped derivation-class
// string reverse.go's reverseValue expects for fd, or "" if reverse.go
// has no conversion for fd's class yet (D-09's unbindable-kind case).
// Reuses fieldmap.go's classify — the identical classification Tier 1's
// forward table already applies to this same field — rather than
// re-deriving field shape a second, possibly-divergent way.
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
// closure never compiles anything — every cel.Program it evaluates was
// already built by buildHookState above.
func (hs *hookState) hook() ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			violations, err := hs.evaluate(m)
			if err != nil {
				// D-12: a reverse-conversion failure (or a genuine CEL
				// evaluation error) is a data-integrity fault, not a
				// protovalidate violation — the mutation fails closed
				// with a plain error, never wrapped as
				// *protovalidate.ValidationError and never assigned a
				// synthesized RuleId.
				return nil, err
			}
			if len(violations) > 0 {
				// D-07 consequence 3: the ONE call site in this package
				// that builds a *protovalidate.ValidationError.
				return nil, newValidationError(violations)
			}
			return next.Mutate(ctx, m)
		})
	}
}

// evaluate runs every compiled residual-CEL program against m's in-scope
// field values, returning the collected *validate.Violation protos.
//
// In-scope is m.Fields() alone, on both Create and Update, with no
// m.Op() branch (D-06; mechanism per 03-RESEARCH.md Pitfall 2 and this
// plan's own tracer_test.go, which proves it empirically against a real
// generated ent.Client): ent's own generated defaults() already
// materializes every Default()-bearing derived field into the mutation
// before any hook runs on Create, so mutation.Fields() already differs
// correctly in content between Create and Update without this hook
// needing to know which operation it is.
func (hs *hookState) evaluate(m ent.Mutation) ([]*validate.Violation, error) {
	if len(hs.evaluators) == 0 {
		return nil, nil
	}

	inScope := make(map[string]struct{}, len(m.Fields()))
	for _, name := range m.Fields() {
		inScope[name] = struct{}{}
	}

	var violations []*validate.Violation
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
		this := val.Interface()

		for _, p := range fe.programs {
			out, _, evalErr := p.prg.Eval(map[string]any{"this": this})
			if evalErr != nil {
				return nil, fmt.Errorf(
					"mixinforproto: %s: evaluating CEL rule %q on field %q: %w",
					hs.msgName, p.ruleID, name, evalErr,
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
// program.go, verified in 03-RESEARCH.md): a bool result of true, or an
// empty string result, means the rule passed; a bool result of false
// produces a violation using p.message if set (falling back to a
// "<ruleID>" returned false message), and a non-empty string result
// produces a violation using that string as the message. Any other
// result type is a genuine evaluator failure (a malformed contract-
// authored expression), not a violation and not a data value — surfaced
// as a plain error, matching D-12's "infrastructure fault, not a
// violation" posture.
func celResultToViolation(fd protoreflect.FieldDescriptor, p celProgram, result any) (*validate.Violation, error) {
	switch v := result.(type) {
	case bool:
		if v {
			return nil, nil
		}
		msg := p.message
		if msg == "" {
			msg = fmt.Sprintf("%q returned false", p.ruleID)
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
