package difftest

import (
	"context"
	"errors"
	"sync"
	"testing"

	"buf.build/go/protovalidate"

	"github.com/smintz/entconnect/mixinforproto/internal/difftest/ent/messagerulecelexpression"
	"github.com/smintz/entconnect/mixinforproto/internal/difftest/ent/messageruleoneof"
	"github.com/smintz/entconnect/mixinforproto/internal/difftest/ent/messagerules"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// This file is Plan 03-05's real-client proof for VAL-08/D-10's
// WithMessageRules(OnCreate) opt-in (mixinforproto/internal/difftest/ent/
// schema/messagerules.go): a real generated ent.Client on the fakeDriver,
// no database, matching D-13's driverless-harness discipline — proving
// the opt-in through ent's REAL withHooks pipeline (PIPE-06), not just
// against hooks_test.go's/messagerules_test.go's hand-built ent.Mutation
// double.

// TestMessageRules_CreateRejectsViolatingEntity is Task 1's Test 2: with
// WithMessageRules(OnCreate) set on the schema, a real Create violating
// "this.lo <= this.hi" is rejected carrying the message rule's own
// RuleId, with no field path (a message-level violation names no single
// field — sortViolations/violationFieldIndex's fieldIndexMessageScoped
// convention, violation.go).
func TestMessageRules_CreateRejectsViolatingEntity(t *testing.T) {
	client := newTestClient(t)

	_, err := client.MessageRules.Create().SetLo(5).SetHi(1).Save(context.Background())
	if err == nil {
		t.Fatal("want an error: lo=5 > hi=1 violates \"this.lo <= this.hi\"")
	}

	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *protovalidate.ValidationError, got %T: %v", err, err)
	}
	if len(ve.Violations) != 1 {
		t.Fatalf("want exactly 1 violation, got %d: %v", len(ve.Violations), ve.Violations)
	}
	const wantRuleID = "messagerules.message_rule_ok.lo_le_hi"
	if got := ve.Violations[0].Proto.GetRuleId(); got != wantRuleID {
		t.Fatalf("want RuleId %q, got %q", wantRuleID, got)
	}
	if got := len(ve.Violations[0].Proto.GetField().GetElements()); got != 0 {
		t.Fatalf("want a message-level violation with no field path elements, got %d: %v", got, ve.Violations[0].Proto.GetField())
	}
}

// TestMessageRules_CreateAcceptsSatisfyingEntity is the accept-side
// counterpart: lo <= hi satisfies the rule and Save returns a literal nil
// error.
func TestMessageRules_CreateAcceptsSatisfyingEntity(t *testing.T) {
	client := newTestClient(t)

	if _, err := client.MessageRules.Create().SetLo(1).SetHi(5).Save(context.Background()); err != nil {
		t.Fatalf("want a nil error for lo=1 <= hi=5 — got: %v", err)
	}
}

// TestMessageRules_UpdateNotSubjectToMessageRuleEnforcement is Task 1's
// Test 3: WithMessageRules(OnCreate) is Create-only — an Update producing
// an entity that would violate "this.lo <= this.hi" if evaluated must
// still succeed, because message-level rules are never evaluated on
// Update under this option (mixinforproto.md §4.3's Create-only opt-in;
// OnUpdateWithFetch is deliberately out of scope for v1 — messagerules.go's
// MessageRuleTrigger doc comment).
func TestMessageRules_UpdateNotSubjectToMessageRuleEnforcement(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	got, err := client.MessageRules.Create().SetLo(1).SetHi(5).Save(ctx)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Setting lo=9 alone (hi stays 5, persisted-elsewhere) would violate
	// "this.lo <= this.hi" if the message-level rule were (wrongly)
	// evaluated on this Update — it must not be.
	if _, err := client.MessageRules.Update().Where(messagerules.IDEQ(got.ID)).SetLo(9).Save(ctx); err != nil {
		t.Fatalf("want a nil error: message-level rules are Create-only under WithMessageRules(OnCreate) — got: %v", err)
	}
}

// TestMessageRules_DeterministicAndConcurrent is Task 1's Test 10: two
// evaluations of the identical Create yield byte-identical violation
// sets (idempotence — the option installs no per-call mutable state),
// and N concurrent Creates against the one compiled hookState produce
// correct, non-interleaved verdicts (go test -race is what actually
// proves the "non-interleaved" half; this test's own assertions prove
// "correct").
func TestMessageRules_DeterministicAndConcurrent(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	t.Run("idempotent", func(t *testing.T) {
		var first *protovalidate.ValidationError
		for i := 0; i < 5; i++ {
			_, err := client.MessageRules.Create().SetLo(5).SetHi(1).Save(ctx)
			var ve *protovalidate.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("run %d: want *protovalidate.ValidationError, got %T: %v", i, err, err)
			}
			if len(ve.Violations) != 1 {
				t.Fatalf("run %d: want exactly 1 violation, got %d: %v", i, len(ve.Violations), ve.Violations)
			}
			if first == nil {
				first = ve
				continue
			}
			if ve.Violations[0].Proto.GetRuleId() != first.Violations[0].Proto.GetRuleId() {
				t.Fatalf("run %d: RuleId changed across runs: %q vs %q", i, ve.Violations[0].Proto.GetRuleId(), first.Violations[0].Proto.GetRuleId())
			}
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		const n = 8
		var wg sync.WaitGroup
		errs := make([]error, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				// Even i: violating (lo=5,hi=1); odd i: satisfying (lo=1,hi=5).
				if i%2 == 0 {
					_, errs[i] = client.MessageRules.Create().SetLo(5).SetHi(1).Save(ctx)
				} else {
					_, errs[i] = client.MessageRules.Create().SetLo(1).SetHi(5).Save(ctx)
				}
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			if i%2 == 0 {
				var ve *protovalidate.ValidationError
				if !errors.As(err, &ve) || len(ve.Violations) != 1 {
					t.Fatalf("goroutine %d (violating): want exactly 1 violation, got err=%v", i, err)
				}
			} else if err != nil {
				t.Fatalf("goroutine %d (satisfying): want a nil error, got %v", i, err)
			}
		}
	})
}

// ============================================================================
// 03-08-PLAN.md Task 2: CR-01 gap closure — the cel_expression and oneof
// MessageRules carriers now flow through the same real ent.Client
// withHooks pipeline (PIPE-06) MessageRuleOk's `cel`-carrier tests above
// already prove for the `cel` carrier. Every case compares violation
// IDENTITY (RuleId, FieldPath) via violationIdentities/sameSet — never a
// bare count — matching this file's and ignore_test.go's own discipline.
// ============================================================================

// TestMessageRuleCelExpression_CreateRejectsViolatingEntity is the
// cel_expression analogue of TestMessageRules_CreateRejectsViolatingEntity
// above: a real Create violating "this.lo <= this.hi" — declared via the
// SIMPLIFIED cel_expression carrier, not `cel` — is rejected. Per this
// plan's own flagged assumption, the expected RuleId is read from
// protovalidate.Validate's OWN output rather than hard-coded: protovalidate
// owns the identifier it assigns a cel_expression rule with no explicit
// id (expressionsToRules, buf.build/go/protovalidate@v1.2.0/builder.go,
// verified this session — the RuleId is the raw expression string itself,
// which differs from celRuleResidual's schema-load-time fingerprint; see
// this plan's SUMMARY for the recorded divergence).
func TestMessageRuleCelExpression_CreateRejectsViolatingEntity(t *testing.T) {
	client := newTestClient(t)

	entity := &mixinforprototestv1.MessageRuleCelExpressionOk{Lo: 5, Hi: 1}
	wantErr := protovalidate.Validate(entity)
	wantIDs, ok := violationIdentities(wantErr)
	if !ok {
		t.Fatalf("want a *protovalidate.ValidationError from the boundary, got %T: %v", wantErr, wantErr)
	}
	if len(wantIDs) != 1 {
		t.Fatalf("want exactly 1 boundary violation, got %v", wantIDs)
	}

	_, err := client.MessageRuleCelExpression.Create().SetLo(5).SetHi(1).Save(context.Background())
	storageIDs, ok := violationIdentities(err)
	if !ok {
		t.Fatalf("want a *protovalidate.ValidationError, got %T: %v", err, err)
	}
	if !sameSet(storageIDs, wantIDs) {
		t.Fatalf("storage/boundary identity mismatch: storage=%v boundary=%v", storageIDs, wantIDs)
	}
}

// TestMessageRuleCelExpression_CreateAcceptsSatisfyingEntity is the
// accept-side counterpart: lo <= hi satisfies the rule.
func TestMessageRuleCelExpression_CreateAcceptsSatisfyingEntity(t *testing.T) {
	client := newTestClient(t)

	if _, err := client.MessageRuleCelExpression.Create().SetLo(1).SetHi(5).Save(context.Background()); err != nil {
		t.Fatalf("want a nil error for lo=1 <= hi=5 — got: %v", err)
	}
}

// TestMessageRuleCelExpression_UpdateNotSubjectToMessageRuleEnforcement is
// the cel_expression analogue of the `cel`-carrier Create-only proof
// above: WithMessageRules(OnCreate) never enforces this carrier on
// Update either.
func TestMessageRuleCelExpression_UpdateNotSubjectToMessageRuleEnforcement(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	got, err := client.MessageRuleCelExpression.Create().SetLo(1).SetHi(5).Save(ctx)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := client.MessageRuleCelExpression.Update().Where(messagerulecelexpression.IDEQ(got.ID)).SetLo(9).Save(ctx); err != nil {
		t.Fatalf("want a nil error: message-level rules are Create-only under WithMessageRules(OnCreate) — got: %v", err)
	}
}

// TestMessageRuleOneof_CreateRejectsViolatingEntity is the oneof
// analogue: MessageRuleOneofOk's rule requires exactly one of lo/hi to
// be set (`required: true`); setting BOTH to non-zero values violates it.
// protovalidate's own fixed "message.oneof" RuleId (verified against
// buf.build/go/protovalidate@v1.2.0/message_oneof.go this session) is
// asserted directly here — unlike cel_expression, this identifier is a
// fixed constant, not contract-author-supplied, so there is no
// hard-coding-vs-read-from-protovalidate distinction to make.
func TestMessageRuleOneof_CreateRejectsViolatingEntity(t *testing.T) {
	client := newTestClient(t)

	entity := &mixinforprototestv1.MessageRuleOneofOk{Lo: 5, Hi: 3}
	wantErr := protovalidate.Validate(entity)
	wantIDs, ok := violationIdentities(wantErr)
	if !ok {
		t.Fatalf("want a *protovalidate.ValidationError from the boundary, got %T: %v", wantErr, wantErr)
	}
	if !wantIDs["message.oneof\x00"] {
		t.Fatalf("want boundary violation identity \"message.oneof\" with no field path, got %v", wantIDs)
	}

	_, err := client.MessageRuleOneof.Create().SetLo(5).SetHi(3).Save(context.Background())
	storageIDs, ok := violationIdentities(err)
	if !ok {
		t.Fatalf("want a *protovalidate.ValidationError, got %T: %v", err, err)
	}
	if !sameSet(storageIDs, wantIDs) {
		t.Fatalf("storage/boundary identity mismatch: storage=%v boundary=%v", storageIDs, wantIDs)
	}
}

// TestMessageRuleOneof_CreateAcceptsSatisfyingEntity is the accept-side
// counterpart: exactly one of lo/hi set (hi left at its proto3 zero)
// satisfies the rule.
func TestMessageRuleOneof_CreateAcceptsSatisfyingEntity(t *testing.T) {
	client := newTestClient(t)

	if _, err := client.MessageRuleOneof.Create().SetLo(5).Save(context.Background()); err != nil {
		t.Fatalf("want a nil error: exactly one of lo/hi is set — got: %v", err)
	}
}

// TestMessageRuleOneof_UpdateNotSubjectToMessageRuleEnforcement is the
// oneof analogue of the Create-only proof above.
func TestMessageRuleOneof_UpdateNotSubjectToMessageRuleEnforcement(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	got, err := client.MessageRuleOneof.Create().SetLo(5).Save(ctx)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Setting hi=3 alone (lo stays 5, persisted-elsewhere) would violate
	// the oneof rule (both non-zero) if it were (wrongly) evaluated on
	// this Update — it must not be.
	if _, err := client.MessageRuleOneof.Update().Where(messageruleoneof.IDEQ(got.ID)).SetHi(3).Save(ctx); err != nil {
		t.Fatalf("want a nil error: message-level rules are Create-only under WithMessageRules(OnCreate) — got: %v", err)
	}
}
