package difftest

import (
	"context"
	"testing"

	"buf.build/go/protovalidate"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// This file is Plan 03-06's real-ent.Client proof for CR-02's gap
// closure: (buf.validate.field).ignore was previously invisible to
// buildHookState's per-field loop, so a field carrying ignore =
// IGNORE_ALWAYS together with a custom (buf.validate.field).cel rule
// produced a divergent verdict between the RPC boundary (which honors
// ignore correctly) and the storage layer (which did not honor it at
// all). Task 1 covers IGNORE_ALWAYS; Task 2 extends this file with
// IGNORE_IF_ZERO_VALUE cases against IgnoreIfZeroWithCel.
//
// Every case here compares violation IDENTITY — the (RuleId, FieldPath)
// set produced by violationIdentities — never a bare count, matching
// this plan's own must_haves.truths/PIPE-06 discipline.

// TestIgnoreAlways_SuppressedFieldProducesZeroViolations is Task 1's
// Test 1 (the tracer's end-to-end assertion): a real Create setting
// always_ignored to a value that FAILS its own cel rule, and enforced to
// a value that PASSES its own rule, succeeds — zero violations, both at
// the storage layer (a real ent.Client Create) and at the boundary (a
// direct protovalidate.Validate call on the identical entity).
func TestIgnoreAlways_SuppressedFieldProducesZeroViolations(t *testing.T) {
	client := newTestClient(t)

	const alwaysIgnoredValue = "nope" // fails "this.startsWith('X')" — must be ignored anyway
	const enforcedValue = "Xok"       // passes "this.startsWith('X')"

	_, err := client.IgnoreAlwaysWithCel.Create().
		SetAlwaysIgnored(alwaysIgnoredValue).
		SetEnforced(enforcedValue).
		Save(context.Background())
	storageIDs, ok := violationIdentities(err)
	if !ok {
		t.Fatalf("want a nil error or a *protovalidate.ValidationError, got %T: %v", err, err)
	}
	if len(storageIDs) != 0 {
		t.Fatalf("want zero storage-layer violations (always_ignored is IGNORE_ALWAYS), got %v", storageIDs)
	}

	entity := &mixinforprototestv1.IgnoreAlwaysWithCel{
		AlwaysIgnored: alwaysIgnoredValue,
		Enforced:      enforcedValue,
	}
	boundaryErr := protovalidate.Validate(entity)
	boundaryIDs, ok := violationIdentities(boundaryErr)
	if !ok {
		t.Fatalf("want a nil error or a *protovalidate.ValidationError from the boundary, got %T: %v", boundaryErr, boundaryErr)
	}
	if !sameSet(storageIDs, boundaryIDs) {
		t.Fatalf("storage/boundary identity mismatch: storage=%v boundary=%v", storageIDs, boundaryIDs)
	}
}

// TestIgnoreAlways_NonIgnoredSiblingStillRejects is Task 1's Test 2: the
// same real Create with enforced set to a FAILING value is rejected, and
// the storage-layer violation-identity set equals the boundary's —
// proving the ignore branch suppressed only always_ignored, not the
// whole message.
func TestIgnoreAlways_NonIgnoredSiblingStillRejects(t *testing.T) {
	client := newTestClient(t)

	const alwaysIgnoredValue = "nope" // fails its own rule — must stay ignored
	const enforcedValue = "nope"      // fails "this.startsWith('X')" — must be rejected

	_, err := client.IgnoreAlwaysWithCel.Create().
		SetAlwaysIgnored(alwaysIgnoredValue).
		SetEnforced(enforcedValue).
		Save(context.Background())
	if err == nil {
		t.Fatal("want an error: enforced fails its own cel rule and is not ignored")
	}
	storageIDs, ok := violationIdentities(err)
	if !ok {
		t.Fatalf("want a *protovalidate.ValidationError, got %T: %v", err, err)
	}

	entity := &mixinforprototestv1.IgnoreAlwaysWithCel{
		AlwaysIgnored: alwaysIgnoredValue,
		Enforced:      enforcedValue,
	}
	boundaryErr := protovalidate.Validate(entity)
	boundaryIDs, ok := violationIdentities(boundaryErr)
	if !ok {
		t.Fatalf("want a *protovalidate.ValidationError from the boundary, got %T: %v", boundaryErr, boundaryErr)
	}
	if !sameSet(storageIDs, boundaryIDs) {
		t.Fatalf("storage/boundary identity mismatch: storage=%v boundary=%v", storageIDs, boundaryIDs)
	}
	const wantRuleID = "ignore.ignore_always_with_cel.enforced.starts_with_x"
	if !storageIDs[wantRuleID+"\x00enforced"] {
		t.Fatalf("want storage violation identity %q, got %v", wantRuleID+"\x00enforced", storageIDs)
	}
}
