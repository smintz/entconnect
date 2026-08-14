package difftest

import (
	"context"
	"errors"
	"testing"

	"buf.build/go/protovalidate"

	"github.com/smintz/entconnect/mixinforproto/internal/difftest/ent/residualcel"
)

// This file is Plan 03-03's end-to-end proof, driven through a real
// generated ent.Client on the fakeDriver — no database, matching D-13's
// driverless-harness discipline. Two things get proven here that no unit
// test of hooks.go/violation.go in isolation can show: that D-07's
// hybrid, including Task 2's dedup resolution to 03-RESEARCH.md Pitfall
// 1, holds through ent's REAL withHooks pipeline on a real Create; and
// that D-06's operation-dependent scope actually behaves as specified
// on a real Update, not just against a hand-built ent.Mutation double
// (hooks_test.go's fakeMutation).

// --- Task 2: the mixed-field convergence proof (PIPE-06's first
// instance) ---

// TestMixedFieldRules_ViolatesOnlyCEL_RealCreate drives the "abc" case
// (satisfies string.min_len=3, does not start with X) through a real
// Create: exactly the custom CEL rule's violation reaches the caller.
func TestMixedFieldRules_ViolatesOnlyCEL_RealCreate(t *testing.T) {
	client := newTestClient(t)

	_, err := client.MixedFieldRules.Create().
		SetBoth("abc").
		SetStandardOnly("valid-length").
		SetCelOnly("Xok").
		Save(context.Background())
	if err == nil {
		t.Fatal("want an error: \"abc\" does not start with X")
	}

	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *protovalidate.ValidationError, got %T: %v", err, err)
	}
	if len(ve.Violations) != 1 {
		t.Fatalf("want exactly 1 violation, got %d: %v", len(ve.Violations), ve.Violations)
	}
	const wantRuleID = "constraints.mixed_field_rules.both.starts_with_x"
	if got := ve.Violations[0].Proto.GetRuleId(); got != wantRuleID {
		t.Fatalf("want RuleId %q, got %q", wantRuleID, got)
	}
	if got := protovalidate.FieldPathString(ve.Violations[0].Proto.GetField()); got != "both" {
		t.Fatalf("want field path %q, got %q", "both", got)
	}
}

// TestMixedFieldRules_ViolatesOnlyStandard_RealCreate drives the "Xy"
// case (starts with X, but only 2 code points, below min_len=3) through
// a real Create: exactly the standard rule's violation reaches the
// caller.
func TestMixedFieldRules_ViolatesOnlyStandard_RealCreate(t *testing.T) {
	client := newTestClient(t)

	_, err := client.MixedFieldRules.Create().
		SetBoth("Xy").
		SetStandardOnly("valid-length").
		SetCelOnly("Xok").
		Save(context.Background())
	if err == nil {
		t.Fatal("want an error: \"Xy\" is below min_len=3")
	}

	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *protovalidate.ValidationError, got %T: %v", err, err)
	}
	if len(ve.Violations) != 1 {
		t.Fatalf("want exactly 1 violation, got %d: %v", len(ve.Violations), ve.Violations)
	}
	const wantRuleID = "string.min_len"
	if got := ve.Violations[0].Proto.GetRuleId(); got != wantRuleID {
		t.Fatalf("want RuleId %q, got %q", wantRuleID, got)
	}
}

// TestMixedFieldRules_ViolatesBoth_RealCreate drives the "ab" case (2
// code points, does not start with X — violates both rules) through a
// real Create: exactly two violations reach the caller, one per rule,
// never a doubled entry for the CEL rule despite it being evaluated by
// both engines internally (Pitfall 1's accepted cost, resolved by
// violation.go's newValidationError before this error ever exists).
func TestMixedFieldRules_ViolatesBoth_RealCreate(t *testing.T) {
	client := newTestClient(t)

	_, err := client.MixedFieldRules.Create().
		SetBoth("ab").
		SetStandardOnly("valid-length").
		SetCelOnly("Xok").
		Save(context.Background())
	if err == nil {
		t.Fatal("want an error: \"ab\" violates both string.min_len and the custom CEL rule")
	}

	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *protovalidate.ValidationError, got %T: %v", err, err)
	}
	if len(ve.Violations) != 2 {
		t.Fatalf("want exactly 2 violations, got %d: %v", len(ve.Violations), ve.Violations)
	}
	seen := map[string]bool{}
	for _, v := range ve.Violations {
		id := v.Proto.GetRuleId()
		if seen[id] {
			t.Fatalf("duplicate RuleId %q in violation set: %v", id, ve.Violations)
		}
		seen[id] = true
	}
	for _, want := range []string{"string.min_len", "constraints.mixed_field_rules.both.starts_with_x"} {
		if !seen[want] {
			t.Fatalf("want RuleId %q present, got %v", want, ve.Violations)
		}
	}
}

// TestMixedFieldRules_SiblingsUnaffected_RealCreate proves the
// non-mixed siblings still behave correctly through a real Create: a bad
// standard_only value alone, and a bad cel_only value alone, each
// produce exactly their own single violation.
func TestMixedFieldRules_SiblingsUnaffected_RealCreate(t *testing.T) {
	client := newTestClient(t)

	t.Run("bad standard_only alone", func(t *testing.T) {
		_, err := client.MixedFieldRules.Create().
			SetBoth("Xvalid").
			SetStandardOnly("ab").
			SetCelOnly("Xok").
			Save(context.Background())
		var ve *protovalidate.ValidationError
		if !errors.As(err, &ve) || len(ve.Violations) != 1 || ve.Violations[0].Proto.GetRuleId() != "string.min_len" {
			t.Fatalf("want exactly 1 string.min_len violation, got %v (err=%v)", ve, err)
		}
	})

	t.Run("bad cel_only alone", func(t *testing.T) {
		_, err := client.MixedFieldRules.Create().
			SetBoth("Xvalid").
			SetStandardOnly("valid-length").
			SetCelOnly("nope").
			Save(context.Background())
		var ve *protovalidate.ValidationError
		if !errors.As(err, &ve) || len(ve.Violations) != 1 || ve.Violations[0].Proto.GetRuleId() != "constraints.mixed_field_rules.cel_only.starts_with_x" {
			t.Fatalf("want exactly 1 cel_only violation, got %v (err=%v)", ve, err)
		}
	})
}

// --- Task 3: D-06's operation-dependent scope, proven on a real Update
// (Pitfall 4's named warning sign is a test suite that only ever
// exercises Create's zero-collapse case — this file's sibling,
// tracer_test.go, already covers Create; the tests below independently
// exercise Update). ---

// TestUpdate_UntouchedDerivedFieldIsNotEvaluated proves D-06/Pitfall 4:
// unlike Create's defaults(), Update has no ent-side safety net that
// materializes a derived field's value into the mutation when the
// caller never calls its setter — so an Update leaving ResidualCel's
// only field ("value") completely untouched must succeed, even though a
// hypothetical persisted-elsewhere empty value would fail
// "this.startsWith('X')" if it were (wrongly) evaluated.
func TestUpdate_UntouchedDerivedFieldIsNotEvaluated(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	got, err := client.ResidualCel.Create().SetValue("Xyz").Save(ctx)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Deliberately no SetValue call: the field is left entirely
	// untouched by the caller, exactly as Pitfall 4 requires this test
	// to independently exercise.
	if _, err := client.ResidualCel.Update().Where(residualcel.IDEQ(got.ID)).Save(ctx); err != nil {
		t.Fatalf("want a nil error: an untouched derived field is out of scope on Update and must not be evaluated — got: %v", err)
	}
}

// TestUpdate_TouchedDerivedFieldIsEvaluated proves D-06's other half: a
// field the caller DID set via Update IS evaluated, exactly like Create.
func TestUpdate_TouchedDerivedFieldIsEvaluated(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	got, err := client.ResidualCel.Create().SetValue("Xyz").Save(ctx)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = client.ResidualCel.Update().Where(residualcel.IDEQ(got.ID)).SetValue("Y").Save(ctx)
	if err == nil {
		t.Fatal("want an error: \"Y\" fails \"this.startsWith('X')\" and was explicitly set on this Update")
	}
	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *protovalidate.ValidationError, got %T: %v", err, err)
	}
	if len(ve.Violations) != 1 {
		t.Fatalf("want exactly 1 violation, got %d: %v", len(ve.Violations), ve.Violations)
	}
	const wantRuleID = "constraints.residual_cel.starts_with_x"
	if got := ve.Violations[0].Proto.GetRuleId(); got != wantRuleID {
		t.Fatalf("want RuleId %q, got %q", wantRuleID, got)
	}
}

// TestUpdate_TouchedDerivedFieldAcceptsValidValue is the accept-side
// counterpart to TestUpdate_TouchedDerivedFieldIsEvaluated: a value that
// DOES satisfy the rule reaches the (fake) driver and Save returns a
// literal nil error.
func TestUpdate_TouchedDerivedFieldAcceptsValidValue(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	got, err := client.ResidualCel.Create().SetValue("Xyz").Save(ctx)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := client.ResidualCel.Update().Where(residualcel.IDEQ(got.ID)).SetValue("Xabc").Save(ctx); err != nil {
		t.Fatalf("want a nil error for \"Xabc\" (satisfies \"this.startsWith('X')\") — got: %v", err)
	}
}
