package mixinforproto

import (
	"sort"
	"testing"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"

	"entgo.io/ent"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// This file proves Task 2's settlement of 03-RESEARCH.md Pitfall 1 (the
// D-07/D-08 hybrid-evaluator convergence risk): mixinforprototestv1.
// MixedFieldRules.both carries BOTH a standard rule (string.min_len = 3)
// and a custom (buf.validate.field).cel rule ("starts with X"). The
// chosen resolution is (a) from Pitfall 1: accept that protovalidate's
// Filter cannot split a field's structural rules from its custom-CEL
// rule (there is no sub-field granularity — field.go's ShouldValidate
// check gates the field's ENTIRE compiled evaluator), so a mixed field's
// CEL rule really is evaluated twice — once by protovalidate's own
// evaluator (scoped in via the Filter alongside every other in-scope
// field), once by the local cel.Env (D-08's residual half) — and
// deduplicate by (RuleId, FieldPath) at violation.go's newValidationError
// before the merged *protovalidate.ValidationError is returned.
//
// Resolution (b) — excluding a mixed field from the standard-rule Filter
// entirely and routing its whole rule set through the local env — was
// considered and rejected: it would require proving a structural rule
// (string.min_len) evaluated exclusively through the local
// pvcel-constructed env produces a byte-identical violation to
// protovalidate's own evaluator's, which is achievable in principle but
// adds a second, parallel structural-rule code path this package does
// not otherwise need. Resolution (a) needs no such proof — it reuses
// protovalidate's own evaluator for structural rules unconditionally
// (D-08's routing-by-kind, unchanged) and pushes the entire convergence
// risk onto ONE well-tested function (newValidationError's dedup), which
// this file exercises directly.

func mustMixedHookState(t *testing.T) *hookState {
	t.Helper()
	md := descriptorOf[*mixinforprototestv1.MixedFieldRules]()
	return mustBuildHookState(t, md)
}

const (
	mixedCelRuleID     = "constraints.mixed_field_rules.both.starts_with_x"
	mixedMinLenRuleID  = "string.min_len"
	mixedCelOnlyRuleID = "constraints.mixed_field_rules.cel_only.starts_with_x"
)

func violationRuleIDs(violations []*validate.Violation) []string {
	out := make([]string, len(violations))
	for i, v := range violations {
		out[i] = v.GetRuleId()
	}
	sort.Strings(out)
	return out
}

// Test 1: a value violating ONLY the custom CEL rule on "both" ("abc" —
// satisfies min_len=3, does not start with X) produces exactly one
// violation, with the custom rule's id.
func TestMixedField_ViolatesOnlyCEL(t *testing.T) {
	hs := mustMixedHookState(t)
	m := newFakeMutation(ent.OpCreate, "MixedFieldRules", map[string]ent.Value{
		"both": "abc",
	})
	violations, err := hs.evaluate(m)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	deduped := dedupeViolations(violations)
	if len(deduped) != 1 {
		t.Fatalf("want exactly 1 violation, got %d: %v", len(deduped), deduped)
	}
	if got := deduped[0].GetRuleId(); got != mixedCelRuleID {
		t.Fatalf("want RuleId %q, got %q", mixedCelRuleID, got)
	}
}

// Test 2: a value violating ONLY the standard rule on "both" ("Xy" —
// starts with X, but only 2 code points, below min_len=3) produces
// exactly one violation with the standard rule's id.
func TestMixedField_ViolatesOnlyStandard(t *testing.T) {
	hs := mustMixedHookState(t)
	m := newFakeMutation(ent.OpCreate, "MixedFieldRules", map[string]ent.Value{
		"both": "Xy",
	})
	violations, err := hs.evaluate(m)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	deduped := dedupeViolations(violations)
	if len(deduped) != 1 {
		t.Fatalf("want exactly 1 violation, got %d: %v", len(deduped), deduped)
	}
	if got := deduped[0].GetRuleId(); got != mixedMinLenRuleID {
		t.Fatalf("want RuleId %q, got %q", mixedMinLenRuleID, got)
	}
}

// Test 3: a value violating BOTH ("ab" — 2 code points, does not start
// with X) produces exactly two violations, one per distinct RuleId, with
// no duplicate (RuleId, FieldPath) pair.
func TestMixedField_ViolatesBoth(t *testing.T) {
	hs := mustMixedHookState(t)
	m := newFakeMutation(ent.OpCreate, "MixedFieldRules", map[string]ent.Value{
		"both": "ab",
	})
	violations, err := hs.evaluate(m)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	deduped := dedupeViolations(violations)
	if len(deduped) != 2 {
		t.Fatalf("want exactly 2 violations, got %d: %v", len(deduped), deduped)
	}
	got := violationRuleIDs(deduped)
	want := []string{mixedCelRuleID, mixedMinLenRuleID}
	sort.Strings(want)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want RuleIds %v, got %v", want, got)
		}
	}
	seen := map[string]bool{}
	for _, v := range deduped {
		k := violationKey(v)
		if seen[k] {
			t.Fatalf("duplicate (RuleId, FieldPath) identity: %s", k)
		}
		seen[k] = true
	}
}

// Test 4 (D-04): the storage-layer violation set for an entity violating
// "both" is equal, by (RuleId, FieldPath) and by message text, to
// protovalidate.Validate on the identical entity message.
// standard_only/cel_only are set to values that satisfy THEIR OWN rules,
// isolating the comparison to "both"'s two violations.
func TestMixedField_MatchesDirectProtovalidateOnEntity(t *testing.T) {
	hs := mustMixedHookState(t)
	m := newFakeMutation(ent.OpCreate, "MixedFieldRules", map[string]ent.Value{
		"both":          "ab",
		"standard_only": "valid-length",
		"cel_only":      "Xvalid",
	})
	violations, err := hs.evaluate(m)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	got := newValidationError(hs.md, violations)

	entity := &mixinforprototestv1.MixedFieldRules{
		Both:         "ab",
		StandardOnly: "valid-length",
		CelOnly:      "Xvalid",
	}
	directErr := protovalidate.Validate(entity)
	var want *protovalidate.ValidationError
	if directErr == nil || !asValidationError(directErr, &want) {
		t.Fatalf("want a *protovalidate.ValidationError from the direct call, got %v", directErr)
	}

	if len(got.Violations) != len(want.Violations) {
		t.Fatalf("violation count mismatch: hook=%d direct=%d (hook=%v direct=%v)",
			len(got.Violations), len(want.Violations), got.Violations, want.Violations)
	}

	toSet := func(vs []*protovalidate.Violation) map[string]string {
		out := make(map[string]string, len(vs))
		for _, v := range vs {
			key := v.Proto.GetRuleId() + "\x00" + protovalidate.FieldPathString(v.Proto.GetField())
			out[key] = v.Proto.GetMessage()
		}
		return out
	}
	gotSet, wantSet := toSet(got.Violations), toSet(want.Violations)
	for k, wantMsg := range wantSet {
		gotMsg, ok := gotSet[k]
		if !ok {
			t.Fatalf("hook is missing violation %q present in direct protovalidate.Validate", k)
		}
		if gotMsg != wantMsg {
			t.Fatalf("message text mismatch for %q: hook=%q direct=%q", k, gotMsg, wantMsg)
		}
	}
	for k := range gotSet {
		if _, ok := wantSet[k]; !ok {
			t.Fatalf("hook produced violation %q not present in direct protovalidate.Validate", k)
		}
	}
}

// Test 5: violations are returned in a stable order (field descriptor
// index, then RuleId); the same rejected mutation run twice yields
// byte-identical error text.
func TestMixedField_StableOrderAcrossRuns(t *testing.T) {
	hs := mustMixedHookState(t)
	newMutation := func() *fakeMutation {
		return newFakeMutation(ent.OpCreate, "MixedFieldRules", map[string]ent.Value{
			"both": "ab",
		})
	}

	var first string
	for i := 0; i < 10; i++ {
		violations, err := hs.evaluate(newMutation())
		if err != nil {
			t.Fatalf("run %d: evaluate: %v", i, err)
		}
		ve := newValidationError(hs.md, violations)
		got := ve.Error()
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("run %d: error text changed:\ngot:  %q\nwant: %q", i, got, first)
		}
	}
}

// Test 6: the sibling field carrying only a standard rule and the
// sibling carrying only a custom rule each behave unchanged — the chosen
// resolution must not regress the non-mixed cases.
func TestMixedField_SiblingsUnaffected(t *testing.T) {
	hs := mustMixedHookState(t)

	t.Run("standard_only violates its min_len rule alone", func(t *testing.T) {
		m := newFakeMutation(ent.OpCreate, "MixedFieldRules", map[string]ent.Value{
			"standard_only": "ab",
		})
		violations, err := hs.evaluate(m)
		if err != nil {
			t.Fatalf("evaluate: %v", err)
		}
		deduped := dedupeViolations(violations)
		if len(deduped) != 1 || deduped[0].GetRuleId() != mixedMinLenRuleID {
			t.Fatalf("want exactly 1 violation with RuleId %q, got %v", mixedMinLenRuleID, deduped)
		}
	})

	t.Run("cel_only violates its cel rule alone", func(t *testing.T) {
		m := newFakeMutation(ent.OpCreate, "MixedFieldRules", map[string]ent.Value{
			"cel_only": "abc",
		})
		violations, err := hs.evaluate(m)
		if err != nil {
			t.Fatalf("evaluate: %v", err)
		}
		deduped := dedupeViolations(violations)
		if len(deduped) != 1 || deduped[0].GetRuleId() != mixedCelOnlyRuleID {
			t.Fatalf("want exactly 1 violation with RuleId %q, got %v", mixedCelOnlyRuleID, deduped)
		}
	})
}

// TestNewValidationError_SingleBuilderSite is a lightweight structural
// pin for the acceptance criterion "violation.go is the only file
// constructing a *protovalidate.ValidationError{" — see hooks.go's own
// single call site (the D-07 consequence 3 constructor) and the
// authoritative, whole-repo version of this assertion: run
// `make check-single-validationerror-site` (scripts/pipeline.sh step 6/6,
// CI's `modules` job). This test only pins that newValidationError itself
// returns a well-formed, non-nil value for a non-empty input, so the
// constructor contract this package relies on cannot silently regress.
func TestNewValidationError_SingleBuilderSite(t *testing.T) {
	hs := mustMixedHookState(t)
	v := &validate.Violation{
		RuleId: strPtr("string.min_len"),
		Field: &validate.FieldPath{
			Elements: []*validate.FieldPathElement{{FieldName: strPtr("both"), FieldNumber: int32Ptr(1)}},
		},
		Message: strPtr("too short"),
	}
	ve := newValidationError(hs.md, []*validate.Violation{v})
	if ve == nil || len(ve.Violations) != 1 {
		t.Fatalf("want a 1-violation ValidationError, got %v", ve)
	}
}

func strPtr(s string) *string { return &s }
func int32Ptr(i int32) *int32 { return &i }

// asValidationError is errors.As, specialized so this file does not need
// to import "errors" purely for one call site alongside protovalidate's
// own error type.
func asValidationError(err error, target **protovalidate.ValidationError) bool {
	if ve, ok := err.(*protovalidate.ValidationError); ok {
		*target = ve
		return true
	}
	return false
}
