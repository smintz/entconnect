package difftest

import (
	"context"
	"errors"
	"testing"

	"buf.build/go/protovalidate"
)

// This file is Plan 03-06 Task 3's real-ent.Client proof for CR-03's gap
// closure: mixinforproto/internal/difftest/ent/schema/
// overriddenrules.go declares OverriddenMixedFieldRules —
// MixedFieldRules (proto/mixinforprototest/v1/constraints.proto, the
// same fixture 03-VERIFICATION.md's own probe used to falsify this) with
// Override("both", field.String("both")) applied. Every case here drives
// a real generated ent.Client, proving through ent's actual withHooks
// pipeline (D-13) — not a hand-built ent.Mutation double — that
// Override(...) suppresses storage-layer validation relay exactly as
// option.go documents.

// TestOptionSuppression_OverriddenFieldAcceptsRuleViolatingValue is the
// gap-closure headline: a real Create setting "both" to "nope" — which
// fails BOTH string.min_len=3 AND "this.startsWith('X')" — succeeds. The
// verifier observed 2 violations for this exact value pre-fix.
func TestOptionSuppression_OverriddenFieldAcceptsRuleViolatingValue(t *testing.T) {
	client := newTestClient(t)

	_, err := client.OverriddenMixedFieldRules.Create().
		SetBoth("nope").
		SetStandardOnly("valid-length").
		SetCelOnly("Xvalid").
		Save(context.Background())
	if err != nil {
		t.Fatalf("want a nil error: \"both\" is overridden and suppresses validation relay entirely — got: %v", err)
	}
}

// TestOptionSuppression_NonOverriddenSiblingStillRejects proves
// suppression is per-field, not per-message: with "both" still set to a
// rule-violating value, a real Create additionally setting "cel_only" to
// a value that fails its own cel rule is rejected, carrying exactly that
// rule's own RuleId — no violation names "both".
func TestOptionSuppression_NonOverriddenSiblingStillRejects(t *testing.T) {
	client := newTestClient(t)

	_, err := client.OverriddenMixedFieldRules.Create().
		SetBoth("nope").
		SetStandardOnly("valid-length").
		SetCelOnly("abc"). // fails "this.startsWith('X')"
		Save(context.Background())
	if err == nil {
		t.Fatal("want an error: cel_only fails its own cel rule and is not overridden")
	}

	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *protovalidate.ValidationError, got %T: %v", err, err)
	}
	if len(ve.Violations) != 1 {
		t.Fatalf("want exactly 1 violation, got %d: %v", len(ve.Violations), ve.Violations)
	}
	const wantRuleID = "constraints.mixed_field_rules.cel_only.starts_with_x"
	if got := ve.Violations[0].Proto.GetRuleId(); got != wantRuleID {
		t.Fatalf("want RuleId %q, got %q", wantRuleID, got)
	}
	if got := protovalidate.FieldPathString(ve.Violations[0].Proto.GetField()); got != "cel_only" {
		t.Fatalf("want field path %q, got %q", "cel_only", got)
	}
}
