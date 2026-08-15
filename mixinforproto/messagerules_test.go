package mixinforproto

import (
	"strings"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/reflect/protoreflect"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// This file is Plan 03-05 Task 1's schema-load-side coverage for
// WithMessageRules(OnCreate)/D-10: the golden-derivation shape of
// MessageRuleOk/MessageRuleExcludedRef (both derive cleanly with no
// options — the "excluded" behavior in their own names only applies when
// a specific test opts a field out via Exclude), and every
// buildHookState-level schema-load behavior the plan's Tests 1 and 4-9
// name. Tests 2/3/10 (the real-Create/real-Update/concurrency proofs)
// live in internal/difftest/messagerules_test.go — they need a real
// generated ent.Client, which this package deliberately never imports.

func TestGolden_MessageRuleOk(t *testing.T) {
	assertGolden(t, "messagerules_ok", derive[*mixinforprototestv1.MessageRuleOk])
}

func TestGolden_MessageRuleExcludedRef(t *testing.T) {
	assertGolden(t, "messagerules_excluded_ref", derive[*mixinforprototestv1.MessageRuleExcludedRef])
}

// --- Task 1, Test 1: without WithMessageRules, a schema over a message
// declaring a cross-field rule loads fine and produces zero storage-layer
// violations for an entity that violates it, including at the threshold
// and one step either side. ---

func TestEvaluate_MessageRuleWithoutOptInProducesZeroViolations(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleOk]()
	hs := mustBuildHookState(t, md) // no WithMessageRules

	for _, tc := range []struct {
		name   string
		lo, hi int32
	}{
		{"lo == hi: satisfies the rule anyway", 5, 5},
		{"lo == hi + 1: violates at the threshold", 6, 5},
		{"lo == hi + 2: violates one step past the threshold", 7, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newFakeMutation(ent.OpCreate, "MessageRuleOk", map[string]ent.Value{
				"lo": tc.lo,
				"hi": tc.hi,
			})
			violations, err := hs.evaluate(m)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if len(violations) != 0 {
				t.Fatalf("want zero violations (message-level rules are boundary-only without the opt-in), got %d: %v", len(violations), violations)
			}
		})
	}
}

// --- Task 1, Test 4: WithMessageRules(OnCreate) + Exclude of a field the
// message rule references fails schema load, first line naming the
// message, the rule id and the offending field. ---

func TestBuildHookState_MessageRuleExcludedRefFailsSchemaLoad(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleExcludedRef]()
	_, err := buildHookState(md, WithMessageRules(OnCreate), Exclude("hi"))
	if err == nil {
		t.Fatal("want an error: the message rule references excluded field \"hi\"")
	}
	const want = `mixinforproto: mixinforprototest.v1.MessageRuleExcludedRef.hi: message rule "messagerules.message_rule_excluded_ref.lo_le_hi" references field "hi", which is excluded via Exclude(...) — stop excluding/overriding "hi", or do not opt into WithMessageRules(OnCreate) for this message`
	firstLine := strings.SplitN(err.Error(), "\n", 2)[0]
	if firstLine != want {
		t.Fatalf("first line mismatch:\n got:  %s\n want: %s", firstLine, want)
	}
}

// --- Task 1, Test 5: two message rules (here: one rule referencing two
// excluded fields) are reported in ONE pass, both named, sorted
// deterministically by field descriptor index. ---

func TestBuildHookState_MessageRuleTwoExcludedRefsFailsInOnePass(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleTwoExcludedRefs]()
	_, err := buildHookState(md, WithMessageRules(OnCreate), Exclude("lo", "hi"))
	if err == nil {
		t.Fatal("want an error: the message rule references two excluded fields")
	}
	msg := err.Error()
	for _, want := range []string{
		"2 failures",
		`field "lo"`,
		`field "hi"`,
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	// D-24: sorted by field descriptor index — "lo" (index 0) before
	// "hi" (index 1).
	if strings.Index(msg, `field "lo"`) > strings.Index(msg, `field "hi"`) {
		t.Fatalf("want \"lo\" reported before \"hi\" (declaration order), got: %s", msg)
	}
}

// --- Task 1, Test 6: a message rule referencing an underivable field
// (a message-typed field with no AsJSON opt-in — mapField skips it
// entirely) fails schema load with the same shape. ---

func TestBuildHookState_MessageRuleUnderivableRefFailsSchemaLoad(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleUnderivableRef]()
	_, err := buildHookState(md, WithMessageRules(OnCreate))
	if err == nil {
		t.Fatal("want an error: the message rule references \"detail\", a message-typed field with no AsJSON opt-in")
	}
	for _, want := range []string{
		"mixinforprototest.v1.MessageRuleUnderivableRef",
		`field "detail"`,
		"derives no ent field at all",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

// --- Task 1, Test 7: WithMessageRules(OnCreate) on a message declaring
// no message-level rules at all is a legal no-op. ---

func TestBuildHookState_MessageRuleNoneIsLegalNoOp(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleNone]()
	hs := mustBuildHookState(t, md, WithMessageRules(OnCreate))
	if len(hs.evaluators) != 0 {
		t.Fatalf("want zero evaluators (no field rules, no message rules), got %d", len(hs.evaluators))
	}
	if !hs.messageRulesOnCreate {
		t.Fatal("want hs.messageRulesOnCreate=true even for a no-op message — the OPTION was passed, only its effect is empty")
	}
}

// --- Task 1, Test 8: a CEL expression containing an identifier that
// merely resembles a field name (a comprehension-local variable, a
// string literal) does NOT trigger a false-positive panic — and,
// stronger than "no panic", the resolved reference set is exactly the
// one real this.<field> select the expression makes. ---

func TestBuildHookState_MessageRuleLookalikeDoesNotFalsePositive(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleLookalike]()
	hs := mustBuildHookState(t, md, WithMessageRules(OnCreate))
	if len(hs.evaluators) != 1 {
		t.Fatalf("want exactly 1 evaluator (only \"hi\" is a real this.<field> reference; \"lo\" is only a comprehension-local variable name and a string-literal substring), got %d", len(hs.evaluators))
	}
	if got := string(hs.evaluators[0].fd.Name()); got != "hi" {
		t.Fatalf("want the one evaluator to be \"hi\", got %q", got)
	}

	// Deeper assertion at the mechanism level, per this file's own doc
	// comment: checkMessageRuleReferences itself resolves exactly {"hi"}.
	o := applyOptions([]Option{WithMessageRules(OnCreate)})
	refs, failures := checkMessageRuleReferences(string(md.FullName()), md, o)
	if len(failures) != 0 {
		t.Fatalf("want zero failures, got %v", failures)
	}
	if len(refs) != 1 || string(refs[0].Name()) != "hi" {
		t.Fatalf("want refs == [hi], got %v", refNames(refs))
	}
}

func refNames(refs []protoreflect.FieldDescriptor) []string {
	out := make([]string, len(refs))
	for i, fd := range refs {
		out[i] = string(fd.Name())
	}
	return out
}

// --- Task 1, Test 9: field-reference matching is byte-exact on
// protoreflect.Name — a JSON-name spelling or a case variant of a real
// field name is not treated as a reference to it, mirroring
// validateOptionNames' matching discipline (derive.go). This is a direct
// property of md.Fields().ByName's own semantics, which
// checkMessageRuleReferences relies on rather than re-implementing —
// asserted here directly rather than via a proto fixture, since CEL's
// own type checker only accepts this.<field> spelled with the real proto
// field name in the first place (a JSON-name or case-variant spelling
// would simply fail to compile, never reach the lookup at all). ---

func TestCheckMessageRuleReferences_FieldLookupIsByteExact(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleOk]()
	for _, name := range []string{"Lo", "LO", "loValue", ""} {
		if fd := md.Fields().ByName(protoreflect.Name(name)); fd != nil {
			t.Fatalf("md.Fields().ByName(%q) resolved to a real field (%v) — want nil for every non-exact spelling", name, fd)
		}
	}
	if fd := md.Fields().ByName(protoreflect.Name("lo")); fd == nil {
		t.Fatal("sanity: md.Fields().ByName(\"lo\") should resolve — the exact proto field name always must")
	}
}

// --- Task 1, Test 10 (schema-load half): buildHookState called twice
// against the same descriptor+options produces the identical
// hs.messageRulesOnCreate/evaluators shape — the mutation-time half
// (idempotent violation sets, concurrent Creates) is proven end-to-end in
// internal/difftest/messagerules_test.go against a real ent.Client. ---

func TestBuildHookState_MessageRuleOkDeterministicAcrossCalls(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleOk]()
	hs1 := mustBuildHookState(t, md, WithMessageRules(OnCreate))
	hs2 := mustBuildHookState(t, md, WithMessageRules(OnCreate))
	if len(hs1.evaluators) != len(hs2.evaluators) {
		t.Fatalf("evaluator count changed across calls: %d vs %d", len(hs1.evaluators), len(hs2.evaluators))
	}
	for i := range hs1.evaluators {
		if hs1.evaluators[i].fd.Name() != hs2.evaluators[i].fd.Name() {
			t.Fatalf("evaluator[%d] field changed across calls: %q vs %q", i, hs1.evaluators[i].fd.Name(), hs2.evaluators[i].fd.Name())
		}
	}
}

// ============================================================================
// 03-08-PLAN.md, Task 1: CR-01 gap closure — the cel_expression and oneof
// MessageRules carriers, previously never read by
// checkMessageRuleReferences.
// ============================================================================

// --- cel_expression carrier: excluded/overridden reference fails schema
// load, naming message/rule/field, exactly like the pre-existing `cel`
// carrier's own MessageRuleExcludedRef fixture. ---

func TestBuildHookState_MessageRuleCelExpressionExcludedRefFailsSchemaLoad(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleCelExpressionExcludedRef]()
	_, err := buildHookState(md, WithMessageRules(OnCreate), Exclude("hi"))
	if err == nil {
		t.Fatal("want an error: the cel_expression message rule references excluded field \"hi\"")
	}
	const want = `mixinforproto: mixinforprototest.v1.MessageRuleCelExpressionExcludedRef.hi: message rule "cel.this.lo <= this.hi" references field "hi", which is excluded via Exclude(...) — stop excluding/overriding "hi", or do not opt into WithMessageRules(OnCreate) for this message`
	firstLine := strings.SplitN(err.Error(), "\n", 2)[0]
	if firstLine != want {
		t.Fatalf("first line mismatch:\n got:  %s\n want: %s", firstLine, want)
	}
}

func TestBuildHookState_MessageRuleCelExpressionOverriddenRefFailsSchemaLoad(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleCelExpressionExcludedRef]()
	_, err := buildHookState(md, WithMessageRules(OnCreate), Override("hi", field.Int32("hi")))
	if err == nil {
		t.Fatal("want an error: the cel_expression message rule references overridden field \"hi\"")
	}
	if !strings.Contains(err.Error(), `field "hi", which is replaced via Override(...)`) {
		t.Fatalf("error %q missing the Override-specific reason", err.Error())
	}
}

// --- oneof carrier: excluded/overridden reference fails schema load,
// naming message/rule/field, using protovalidate's own fixed
// "message.oneof" RuleId as the rule identifier (this carrier has no `id`
// field of its own — see messageRuleRef.ruleID's doc comment). ---

func TestBuildHookState_MessageRuleOneofExcludedRefFailsSchemaLoad(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleOneofExcludedRef]()
	_, err := buildHookState(md, WithMessageRules(OnCreate), Exclude("hi"))
	if err == nil {
		t.Fatal("want an error: the oneof message rule references excluded field \"hi\"")
	}
	const want = `mixinforproto: mixinforprototest.v1.MessageRuleOneofExcludedRef.hi: message rule "message.oneof" references field "hi", which is excluded via Exclude(...) — stop excluding/overriding "hi", or do not opt into WithMessageRules(OnCreate) for this message`
	firstLine := strings.SplitN(err.Error(), "\n", 2)[0]
	if firstLine != want {
		t.Fatalf("first line mismatch:\n got:  %s\n want: %s", firstLine, want)
	}
}

func TestBuildHookState_MessageRuleOneofOverriddenRefFailsSchemaLoad(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleOneofExcludedRef]()
	_, err := buildHookState(md, WithMessageRules(OnCreate), Override("hi", field.Int32("hi")))
	if err == nil {
		t.Fatal("want an error: the oneof message rule references overridden field \"hi\"")
	}
	if !strings.Contains(err.Error(), `field "hi", which is replaced via Override(...)`) {
		t.Fatalf("error %q missing the Override-specific reason", err.Error())
	}
}

// --- Both alternate carriers' happy paths seed refs (and therefore
// buildHookState's extraFields), sorted by field descriptor index, just
// like the `cel` carrier already does. ---

func TestBuildHookState_MessageRuleCelExpressionRefsSeedExtraFields(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleCelExpressionOk]()
	o := applyOptions([]Option{WithMessageRules(OnCreate)})
	refs, failures := checkMessageRuleReferences(string(md.FullName()), md, o)
	if len(failures) != 0 {
		t.Fatalf("want zero failures, got %v", failures)
	}
	if len(refs) != 2 || string(refs[0].Name()) != "lo" || string(refs[1].Name()) != "hi" {
		t.Fatalf("want refs == [lo, hi] (sorted by descriptor index), got %v", refNames(refs))
	}
}

func TestBuildHookState_MessageRuleOneofRefsSeedExtraFields(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleOneofOk]()
	o := applyOptions([]Option{WithMessageRules(OnCreate)})
	refs, failures := checkMessageRuleReferences(string(md.FullName()), md, o)
	if len(failures) != 0 {
		t.Fatalf("want zero failures, got %v", failures)
	}
	if len(refs) != 2 || string(refs[0].Name()) != "lo" || string(refs[1].Name()) != "hi" {
		t.Fatalf("want refs == [lo, hi] (sorted by descriptor index), got %v", refNames(refs))
	}
}

// --- Offenders from TWO DIFFERENT carriers on the SAME message are
// reported in one buildHookState call (one derivationError), sorted
// deterministically — proving messageRuleReferences' normalization
// really is one convergence point, not two carriers each doing their own
// thing. Rerun via `go test -count=5` (Task 1's own <verify>) to catch
// any map-iteration-order dependency. ---

func TestBuildHookState_MessageRuleMultiCarrierExcludedRefFailsInOnePass(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleMultiCarrierExcludedRef]()
	_, err := buildHookState(md, WithMessageRules(OnCreate), Exclude("hi"))
	if err == nil {
		t.Fatal("want an error: both the cel_expression and oneof rules reference excluded field \"hi\"")
	}
	msg := err.Error()
	for _, want := range []string{
		"2 failures",
		`"cel.this.lo <= this.hi" references field "hi"`,
		`"message.oneof" references field "hi"`,
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	// D-24: sorted by field descriptor index (both failures name "hi",
	// tied), then by rule ("WithMessageRules", tied), then by
	// description — "cel." sorts before "message." lexically.
	if strings.Index(msg, "cel.this.lo <= this.hi") > strings.Index(msg, "message.oneof") {
		t.Fatalf("want the cel_expression failure reported before the oneof failure (D-24 description-order tiebreak), got: %s", msg)
	}
}

// --- oneof-specific: a name in the rule's `fields` list that resolves to
// no real field is a collected failure, never a silent skip — unlike the
// CEL carriers, `oneof` has no compiler guaranteeing every name is real.
// Exercised directly against messageRuleReferences (the normalization
// helper), which needs no proto fixture of its own: it accepts an
// explicit, hand-built *validate.MessageRules rather than resolving one
// from md's own declared options. ---

func TestMessageRuleReferences_OneofUnknownFieldNameIsCollectedFailure(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleOk]() // has "lo"/"hi", no message-level rules of its own
	env, err := messageRuleCELEnv(md)
	if err != nil {
		t.Fatalf("messageRuleCELEnv: %v", err)
	}
	msgRules := &validate.MessageRules{
		Oneof: []*validate.MessageOneofRule{
			{Fields: []string{"lo", "does_not_exist"}},
		},
	}
	refs, failures := messageRuleReferences(md, msgRules, env)
	if len(failures) != 1 {
		t.Fatalf("want exactly 1 failure (the unresolvable name), got %d: %v", len(failures), failures)
	}
	if failures[0].field != "does_not_exist" {
		t.Fatalf("want the failure to name field %q, got %q", "does_not_exist", failures[0].field)
	}
	if !strings.Contains(failures[0].description, "message.oneof") || !strings.Contains(failures[0].description, "does not declare") {
		t.Fatalf("failure description %q missing expected content", failures[0].description)
	}
	// The resolvable name ("lo") still surfaces as a real reference —
	// one bad name in a rule's fields list doesn't silently drop the
	// good ones.
	if len(refs) != 1 || refs[0].carrier != "oneof" || len(refs[0].fieldNames) != 1 || refs[0].fieldNames[0] != "lo" {
		t.Fatalf("want one oneof ref naming only \"lo\", got %+v", refs)
	}
}

// ============================================================================
// 03-08-PLAN.md, Task 2: WR-04 gap closure — WithMessageRules honors its
// trigger instead of discarding it.
// ============================================================================

// TestWithMessageRules_OnCreateStillWorks is the regression guard: passing
// the one declared trigger value behaves exactly as before this gap
// closure.
func TestWithMessageRules_OnCreateStillWorks(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleOk]()
	hs := mustBuildHookState(t, md, WithMessageRules(OnCreate))
	if !hs.messageRulesOnCreate {
		t.Fatal("want hs.messageRulesOnCreate=true for WithMessageRules(OnCreate)")
	}
}

// TestWithMessageRules_UndeclaredTriggerFailsSchemaLoad is WR-04's core
// proof: an undeclared MessageRuleTrigger value fails schema load, naming
// WithMessageRules and the offending numeric value, rather than silently
// behaving as OnCreate. Exercised against MessageRuleNone (no message-
// level rules at all) specifically to prove the check fires independently
// of whether the message has any rules to enumerate — the trigger itself
// is invalid regardless of what it would have gated.
func TestWithMessageRules_UndeclaredTriggerFailsSchemaLoad(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleNone]()
	_, err := buildHookState(md, WithMessageRules(MessageRuleTrigger(7)))
	if err == nil {
		t.Fatal("want an error: MessageRuleTrigger(7) is not a declared trigger value")
	}
	for _, want := range []string{"WithMessageRules", "7"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestWithMessageRules_UndeclaredTriggerFailsEvenWithRealRules proves the
// same failure against a message that DOES declare message-level rules
// (MessageRuleOk), so the undeclared-trigger check is proven to run
// BEFORE any rule-reference walk, not merely on the no-rule fast path.
func TestWithMessageRules_UndeclaredTriggerFailsEvenWithRealRules(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleOk]()
	_, err := buildHookState(md, WithMessageRules(MessageRuleTrigger(99)))
	if err == nil {
		t.Fatal("want an error: MessageRuleTrigger(99) is not a declared trigger value")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Fatalf("error %q missing the offending trigger value", err.Error())
	}
}

// --- VAL-08/empty edge probe: a cel_expression rule that compiles but
// makes no this.<field> select at all is a legal no-op — schema load
// succeeds under WithMessageRules(OnCreate) and seeds no extraFields,
// exactly like MessageRuleNone (Test 7). ---

func TestBuildHookState_MessageRuleCelExpressionNoFieldRefIsLegalNoOp(t *testing.T) {
	md := descriptorOf[*mixinforprototestv1.MessageRuleCelExpressionNoFieldRef]()
	hs := mustBuildHookState(t, md, WithMessageRules(OnCreate))
	if len(hs.evaluators) != 0 {
		t.Fatalf("want zero evaluators (the rule references no field, and \"lo\" itself carries no rule of its own), got %d", len(hs.evaluators))
	}
	if !hs.messageRulesOnCreate {
		t.Fatal("want hs.messageRulesOnCreate=true even for this no-field-ref rule — the OPTION was passed, only its field-reference effect is empty")
	}

	o := applyOptions([]Option{WithMessageRules(OnCreate)})
	refs, failures := checkMessageRuleReferences(string(md.FullName()), md, o)
	if len(failures) != 0 {
		t.Fatalf("want zero failures, got %v", failures)
	}
	if len(refs) != 0 {
		t.Fatalf("want zero refs (no this.<field> select), got %v", refNames(refs))
	}
}
