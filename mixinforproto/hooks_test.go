package mixinforproto

import (
	"errors"
	"math"
	"sort"
	"strings"
	"testing"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"

	"entgo.io/ent"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// fakeMutation is a minimal ent.Mutation double: embedding the (nil) ent.Mutation
// interface promotes every method this test package never calls, so only
// Op/Type/Fields/Field need real implementations — any unimplemented method
// called by accident panics loudly (nil interface method call) rather than
// silently returning a zero value, which is the property that makes Test 7
// below ("evaluate calls no evaluator") a real proof rather than an assumption.
type fakeMutation struct {
	ent.Mutation
	op     ent.Op
	typ    string
	fields []string
	values map[string]ent.Value
}

// newFakeMutation builds a fakeMutation whose Fields() is exactly the
// sorted key set of values — the test's own chosen "in scope" set,
// independent of whichever real ent Create/Update semantics would have
// produced it (D-06's operation-dependent scope is exercised end-to-end
// by internal/difftest's real generated ent.Client instead; this
// package's unit tests exercise hooks.go's field-scope CONSUMPTION, not
// ent's own defaults()/UpdateDefault mechanics).
func newFakeMutation(op ent.Op, typ string, values map[string]ent.Value) *fakeMutation {
	fields := make([]string, 0, len(values))
	for k := range values {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	return &fakeMutation{op: op, typ: typ, fields: fields, values: values}
}

func (m *fakeMutation) Op() ent.Op       { return m.op }
func (m *fakeMutation) Type() string     { return m.typ }
func (m *fakeMutation) Fields() []string { return m.fields }
func (m *fakeMutation) Field(name string) (ent.Value, bool) {
	v, ok := m.values[name]
	return v, ok
}

var _ ent.Mutation = (*fakeMutation)(nil)

func mustBuildHookState(t *testing.T, md protoreflect.MessageDescriptor) *hookState {
	t.Helper()
	hs, err := buildHookState(md)
	if err != nil {
		t.Fatalf("buildHookState(%s): %v", md.FullName(), err)
	}
	return hs
}

func descriptorOf[M proto.Message]() protoreflect.MessageDescriptor {
	return (*new(M)).ProtoReflect().Descriptor()
}

// singleViolationField returns the field name of violations[0]'s first
// (and, for this package's own violations, only) field path element, or
// "" if violations is empty or the violation carries no field path.
func singleViolationField(v *validate.Violation) string {
	els := v.GetField().GetElements()
	if len(els) == 0 {
		return ""
	}
	return els[0].GetFieldName()
}

// --- Task 1, Test 1: a Tier-1-translated rule (string.max_len) failing at
// the storage layer carries protovalidate's own constraint id. ---

func TestEvaluate_StandardRule_StringMaxLenCarriesProtovalidateRuleID(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.StringCodePointBounds]()
	hs := mustBuildHookState(t, md)

	// Only max_len_field (max_len=5) is in scope, isolating the
	// violation to that one field — "abcdef" is 6 code points.
	m := newFakeMutation(ent.OpCreate, "StringCodePointBounds", map[string]ent.Value{
		"max_len_field": "abcdef",
	})
	violations, err := hs.evaluate(m)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("want exactly 1 violation, got %d: %v", len(violations), violations)
	}
	v := violations[0]
	if got := v.GetRuleId(); got != "string.max_len" {
		t.Fatalf("want RuleId %q, got %q", "string.max_len", got)
	}
	if got := singleViolationField(v); got != "max_len_field" {
		t.Fatalf("want field path %q, got %q", "max_len_field", got)
	}
}

// --- Task 1, Test 2: a standard rule Tier 1 declined to translate
// (float.gt) yields matching verdicts at the value exactly one
// representable unit either side of the bound, against a direct
// protovalidate.Validate call on the same entity message (D-04). ---

func TestEvaluate_StandardRule_FloatGtMatchesDirectProtovalidateAtBoundary(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.FloatComparators]()
	hs := mustBuildHookState(t, md)

	const bound float32 = 1.5
	below := math.Nextafter32(bound, float32(math.Inf(-1))) // one unit below: fails gt
	above := math.Nextafter32(bound, float32(math.Inf(1)))  // one unit above: passes gt

	for _, tc := range []struct {
		name    string
		value   float32
		wantErr bool
	}{
		{"one unit below the bound", below, true},
		{"exactly at the bound", bound, true}, // gt is strict: equal fails
		{"one unit above the bound", above, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newFakeMutation(ent.OpCreate, "FloatComparators", map[string]ent.Value{
				"gt_field": tc.value,
			})
			violations, err := hs.evaluate(m)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			hookRejected := len(violations) > 0

			// LtField/GteLteField are set to values that satisfy THEIR OWN
			// rules (lt: 9.5; gte/lte: [1.5, 9.5]) so the direct
			// protovalidate.Validate comparison below isolates the
			// verdict to gt_field's own rule — a zero-valued
			// GteLteField would otherwise fail its own gte:1.5 bound
			// independently of gt_field, corrupting the comparison.
			entity := &mixinforprototestv1.FloatComparators{
				GtField:     tc.value,
				LtField:     0,
				GteLteField: 5,
			}
			directErr := protovalidate.Validate(entity)
			directRejected := directErr != nil

			if hookRejected != directRejected {
				t.Fatalf("verdict mismatch for value %v: hook rejected=%v (violations=%v), direct protovalidate rejected=%v (%v)",
					tc.value, hookRejected, violations, directRejected, directErr)
			}
			if hookRejected != tc.wantErr {
				t.Fatalf("value %v: want rejected=%v, got %v", tc.value, tc.wantErr, hookRejected)
			}
		})
	}
}

// --- Task 1, Test 3: required-on-a-non-presence field produces the same
// verdict at the storage layer that protovalidate.Validate produces for
// the same entity message (D-04). ---

func TestEvaluate_StandardRule_RequiredOnNonPresenceMatchesDirectProtovalidate(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.RequiredPlainNonString]()
	hs := mustBuildHookState(t, md)

	for _, tc := range []struct {
		name    string
		value   int32
		wantErr bool
	}{
		{"zero value: Has() false, required violation", 0, true},
		{"non-zero value: Has() true, satisfies required", 5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newFakeMutation(ent.OpCreate, "RequiredPlainNonString", map[string]ent.Value{
				"value": tc.value,
			})
			violations, err := hs.evaluate(m)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			hookRejected := len(violations) > 0

			entity := &mixinforprototestv1.RequiredPlainNonString{Value: tc.value}
			directErr := protovalidate.Validate(entity)
			directRejected := directErr != nil

			if hookRejected != directRejected {
				t.Fatalf("verdict mismatch for value %d: hook rejected=%v, direct protovalidate rejected=%v (%v)",
					tc.value, hookRejected, directRejected, directErr)
			}
			if hookRejected != tc.wantErr {
				t.Fatalf("value %d: want rejected=%v, got %v", tc.value, tc.wantErr, hookRejected)
			}
			if hookRejected && violations[0].GetRuleId() != "required" {
				t.Fatalf("want RuleId %q, got %q", "required", violations[0].GetRuleId())
			}
		})
	}
}

// --- Task 1, Test 4: message-level (cross-field) rules produce ZERO
// storage-layer violations when WithMessageRules was not passed, even for
// an entity whose cross-field rule would fail (VAL-08). No corpus .proto
// fixture carries a message-level rule, so this test builds one directly
// via protodesc — a real protoreflect.MessageDescriptor, not a mock —
// with one field-level `required` rule (so hs.evaluators is non-empty and
// the hook actually runs) plus a message-level CEL rule that a real
// entity ALWAYS violates (lo > hi). ---

func buildMessageRuleTestDescriptor(t *testing.T, fileSuffix string) protoreflect.MessageDescriptor {
	t.Helper()

	loOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(loOpts, validate.E_Field, &validate.FieldRules{
		Required: proto.Bool(true),
	})

	msgOpts := &descriptorpb.MessageOptions{}
	proto.SetExtension(msgOpts, validate.E_Message, &validate.MessageRules{
		Cel: []*validate.Rule{
			{
				Id:         proto.String("hookstest.lo_gt_hi"),
				Message:    proto.String("lo must not exceed hi"),
				Expression: proto.String("this.lo <= this.hi"),
			},
		},
	})

	fdProto := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("hooks_test/message_rule_" + fileSuffix + ".proto"),
		Package:    proto.String("mixinforprototest.hookstest.v1"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"buf/validate/validate.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name:    proto.String("LoHi"),
				Options: msgOpts,
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:    proto.String("lo"),
						Number:  proto.Int32(1),
						Label:   descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:    descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Options: loOpts,
					},
					{
						Name:   proto.String("hi"),
						Number: proto.Int32(2),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
					},
				},
			},
		},
	}

	file, err := protodesc.NewFile(fdProto, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatalf("protodesc.NewFile: %v", err)
	}
	return file.Messages().Get(0)
}

func TestEvaluate_MessageLevelRulesStayBoundaryOnlyByDefault(t *testing.T) {
	md := buildMessageRuleTestDescriptor(t, "test4")
	hs := mustBuildHookState(t, md)

	// lo=5, hi=1: violates the message-level "this.lo <= this.hi" rule.
	// lo is non-zero, so the field-level `required` rule is satisfied —
	// isolating the assertion to the message-level rule's absence.
	m := newFakeMutation(ent.OpCreate, "LoHi", map[string]ent.Value{
		"lo": int32(5),
		"hi": int32(1),
	})
	violations, err := hs.evaluate(m)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want zero violations (message-level rules are boundary-only by default), got %d: %v", len(violations), violations)
	}
}

// --- Task 1, Test 5: a FieldMask-gated Update touching only one field
// produces no violation for an untouched field carrying `required` — the
// phantom-violation case D-03 exists to make structurally impossible. ---

func TestEvaluate_FieldMaskUpdate_UntouchedRequiredSiblingProducesNoViolation(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.StringFormatWithSibling]()
	hs := mustBuildHookState(t, md)

	// Only "endpoint" is in scope, as a FieldMask-gated Update would
	// produce; "owner" (required) is deliberately left out of m.Fields()
	// even though a real persisted owner value might be empty.
	m := newFakeMutation(ent.OpUpdateOne, "StringFormatWithSibling", map[string]ent.Value{
		"endpoint": "https://example.com",
	})
	violations, err := hs.evaluate(m)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want zero violations (owner is out of scope), got %d: %v", len(violations), violations)
	}
}

// --- Task 1, Test 6: the protovalidate.Validator is constructed once at
// Hooks()-construction time; running N mutations performs no further
// construction. ---

func TestBuildHookState_StandardValidatorBuiltOnceAtConstruction(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.StringCodePointBounds]()

	before := StandardValidatorBuildCount()
	hs1 := mustBuildHookState(t, md)
	afterFirst := StandardValidatorBuildCount()
	if afterFirst != before+1 {
		t.Fatalf("want exactly 1 validator build from the first buildHookState call, got delta %d", afterFirst-before)
	}
	hs2 := mustBuildHookState(t, md)
	afterSecond := StandardValidatorBuildCount()
	if afterSecond != afterFirst+1 {
		t.Fatalf("want exactly 1 validator build from the second buildHookState call, got delta %d", afterSecond-afterFirst)
	}

	for i := 0; i < 5; i++ {
		m := newFakeMutation(ent.OpCreate, "StringCodePointBounds", map[string]ent.Value{
			"max_len_field": "abc",
			"min_len_field": "abcde",
			"len_field":     "abcd",
		})
		if _, err := hs1.evaluate(m); err != nil {
			t.Fatalf("hs1.evaluate #%d: %v", i, err)
		}
		if _, err := hs2.evaluate(m); err != nil {
			t.Fatalf("hs2.evaluate #%d: %v", i, err)
		}
	}
	afterMutations := StandardValidatorBuildCount()
	if afterMutations != afterSecond {
		t.Fatalf("want validator build count unchanged by running mutations (before=%d, after=%d)", afterSecond, afterMutations)
	}
}

// --- Task 1, Test 7: for a message with zero in-scope fields, the hook
// returns a nil error and calls no evaluator. Proven empirically by
// nulling out hs.validator after construction: if evaluate() incorrectly
// attempted to call it despite an empty in-scope set, this test would
// panic (a nil-interface method call), not merely return an unexpected
// result. ---

func TestEvaluate_EmptyInScopeCallsNoEvaluator(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.StringCodePointBounds]()
	hs := mustBuildHookState(t, md)
	hs.validator = nil // see doc comment: proves no call is attempted.

	m := newFakeMutation(ent.OpUpdateOne, "StringCodePointBounds", map[string]ent.Value{
		// "irrelevant" is not one of StringCodePointBounds' fields, so no
		// entry in hs.evaluators matches it — the in-scope set (relative
		// to hs.evaluators) is empty even though m.Fields() is not.
		"irrelevant": "value",
	})
	violations, err := hs.evaluate(m)
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if violations != nil {
		t.Fatalf("want nil violations, got %v", violations)
	}
}

// TestBuildHookState_NoRulesProducesNoEvaluators proves the sibling empty
// edge at construction time: a message with no protovalidate rules
// anywhere never builds a standard-rule validator at all (mixin.go's
// len(hs.evaluators) == 0 check then skips installing a hook).
func TestBuildHookState_NoRulesProducesNoEvaluators(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.NoRules]()
	hs := mustBuildHookState(t, md)
	if len(hs.evaluators) != 0 {
		t.Fatalf("want zero evaluators for a constraint-free message, got %d", len(hs.evaluators))
	}
	if hs.validator != nil {
		t.Fatal("want no standard-rule validator built for a constraint-free message")
	}
}

// --- Task 3: an uncompilable CEL expression fails at schema load. Since
// buf itself never rejects an arbitrary custom CEL string at build time
// (unlike string.pattern's predefined RE2 check — see constraints.proto's
// StringPattern doc comment), this exercises compileCELRule directly
// against a hand-built, syntactically invalid *validate.Rule — the same
// technique validate_test.go's TestPatternCompiles uses for a failure
// mode buf's own lint makes unreachable through a real corpus fixture. ---

func TestCompileCELRule_UncompilableExpressionIsAnError(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.ResidualCel]()
	fd := md.Fields().ByName("value")
	if fd == nil {
		t.Fatal("want a \"value\" field on ResidualCel")
	}
	r := &validate.Rule{
		Id:         proto.String("hookstest.uncompilable"),
		Expression: proto.String("this.startsWith('X'"), // unbalanced parens
	}
	_, err := compileCELRule(fd, r)
	if err == nil {
		t.Fatal("want an error for an uncompilable CEL expression")
	}
	for _, want := range []string{"mixinforprototest.v1.ResidualCel", "value", "hookstest.uncompilable"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestBuildHookState_UncompilableCELExpressionsAreCollectedInOnePass
// builds a two-field message via protodesc, both fields carrying a
// syntactically invalid CEL expression, and asserts buildHookState
// reports BOTH offenders in a single derivationError (D-09's "report all
// offenders in one pass" discipline, inherited from Phase 1 D-09) rather
// than stopping at the first.
func TestBuildHookState_UncompilableCELExpressionsAreCollectedInOnePass(t *testing.T) {
	fieldOpts := func(exprID string) *descriptorpb.FieldOptions {
		opts := &descriptorpb.FieldOptions{}
		proto.SetExtension(opts, validate.E_Field, &validate.FieldRules{
			Cel: []*validate.Rule{
				{
					Id:         proto.String(exprID),
					Expression: proto.String("this.startsWith('X'"), // unbalanced parens
				},
			},
		})
		return opts
	}

	fdProto := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("hooks_test/two_bad_cel.proto"),
		Package:    proto.String("mixinforprototest.hookstest.v1"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"buf/validate/validate.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("TwoBadCel"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:    proto.String("first"),
						Number:  proto.Int32(1),
						Label:   descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:    descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Options: fieldOpts("hookstest.first_bad"),
					},
					{
						Name:    proto.String("second"),
						Number:  proto.Int32(2),
						Label:   descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:    descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Options: fieldOpts("hookstest.second_bad"),
					},
				},
			},
		},
	}
	file, err := protodesc.NewFile(fdProto, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatalf("protodesc.NewFile: %v", err)
	}
	md := file.Messages().Get(0)

	_, err = buildHookState(md)
	if err == nil {
		t.Fatal("want an error for two uncompilable CEL expressions")
	}
	msg := err.Error()
	for _, want := range []string{
		"mixinforprototest.hookstest.v1.TwoBadCel",
		"first", "hookstest.first_bad",
		"second", "hookstest.second_bad",
		"2", // the failure count, per D-09's first-line discipline
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}

	var derr *derivationError
	if !errors.As(err, &derr) {
		t.Fatalf("want a *derivationError, got %T", err)
	}
}
