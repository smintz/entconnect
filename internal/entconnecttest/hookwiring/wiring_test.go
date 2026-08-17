// Package hookwiring — see ordering_test.go's package doc. This file is
// D-13's real-client wiring proof: a real in-memory sqlite-backed
// ent.Client, mounted behind the generated entconnect server wiring on a
// real httptest.NewServer, driven through a real generated Connect client.
// 03-CONTEXT.md D-13 requires two harnesses deliberately — the driverless
// mixinforproto/internal/difftest sweep proves the HOOK'S OWN verdicts;
// only this real ent client behind a real Connect handler proves the
// WIRING — that ent genuinely calls the hook in the real mutation path,
// not merely that the hook produces correct output when called directly.
//
// D-04: comparisons against protovalidate.Validate MUST be entity-relative
// (against a bare *entconnecttestv1.HookWiring), never against the request
// wrapper (*entconnecttestv1.CreateHookWiringRequest) — the wrapper's own
// violations carry a "hook_wiring."-prefixed field path the storage layer
// (which only ever sees the entity, never the wrapper) cannot and should
// not reproduce.
package hookwiring

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"

	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"

	"github.com/smintz/entconnect/internal/entconnecttest/hookwiring/ent"
	"github.com/smintz/entconnect/internal/entconnecttest/hookwiring/entconnect"
	"github.com/smintz/entconnect/runtime/viewer"
)

// testAuthenticator is a test-only runtime.Authenticator — mirrors
// internal/entconnecttest/{update,write}'s own.
type testAuthenticator struct {
	subject string
}

func (a testAuthenticator) AuthenticateAndViewer(ctx context.Context, _ http.Header) (context.Context, error) {
	return viewer.NewContext(ctx, testViewer{subject: a.subject}), nil
}

// newTestServer builds a real HTTP server mounting the generated
// HookWiringCreateService wiring behind authenticator, and returns a real
// Connect client pointed at it — mirrors
// internal/entconnecttest/{update,write}'s own newTestServer.
func newTestServer(t *testing.T, client *ent.Client, authenticator testAuthenticator) entconnecttestv1connect.HookWiringCreateServiceClient {
	t.Helper()
	srv, err := entconnect.NewServer(client, authenticator)
	if err != nil {
		t.Fatalf("entconnect.NewServer: %v", err)
	}
	mux := http.NewServeMux()
	srv.Register(mux)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return entconnecttestv1connect.NewHookWiringCreateServiceClient(ts.Client(), ts.URL)
}

// violationIdentity returns v's (RuleId, FieldPath) identity, in the same
// shape mixinforproto/violation.go's own (unexported) violationKey uses —
// RuleId plus protovalidate's own exported FieldPathString encoding, so
// two violations naming the same field agree on the identical string
// regardless of which evaluator produced them.
func violationIdentity(v *protovalidate.Violation) string {
	return v.Proto.GetRuleId() + "\x00" + protovalidate.FieldPathString(v.Proto.GetField())
}

// violationIdentitySet returns ve's violations as a set of (RuleId,
// FieldPath) identities, for an order-independent comparison between two
// independently-produced *protovalidate.ValidationError values.
func violationIdentitySet(ve *protovalidate.ValidationError) map[string]bool {
	out := make(map[string]bool, len(ve.Violations))
	for _, v := range ve.Violations {
		out[violationIdentity(v)] = true
	}
	return out
}

// TestWiring_ConnectCreateRejectsInvalidPayload is Test 1: a real Connect
// Create request whose payload violates cel_field's residual rule is
// answered CodeInvalidArgument — proving the whole stack (boundary and
// storage layer alike, both armed for the identical contract rule) rejects
// end to end over real HTTP.
func TestWiring_ConnectCreateRejectsInvalidPayload(t *testing.T) {
	client := newTestClient(t)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	_, err := c.CreateHookWiring(context.Background(), connect.NewRequest(&entconnecttestv1.CreateHookWiringRequest{
		HookWiring: &entconnecttestv1.HookWiring{CelField: "nope", StandardField: "abc"},
	}))
	if err == nil {
		t.Fatal("want an error for an invalid cel_field, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
}

// TestWiring_StorageLayerRejectsIndependentlyAndMatchesEntityValidation is
// Test 2 plus the RuleId/FieldPath identity requirement: driving the ent
// client DIRECTLY (no HTTP, no interceptor chain — the boundary is never
// in the picture) for the identical invalid input is STILL rejected, and
// the storage-layer *protovalidate.ValidationError's violations carry
// EXACTLY the (RuleId, FieldPath) identities protovalidate.Validate
// produces for the equivalent bare entity message (D-04: entity-relative,
// field path included in the comparison — never excluded, and never
// compared against the request-wrapper's own wrapper-prefixed path).
// This is D-13's core proof: ent genuinely invokes the mixin hook in the
// real mutation path — a regression that silently stopped calling the
// hook would make this test's "err == nil" branch fire instead.
func TestWiring_StorageLayerRejectsIndependentlyAndMatchesEntityValidation(t *testing.T) {
	client := newTestClient(t)

	entity := &entconnecttestv1.HookWiring{CelField: "nope", StandardField: "abc"}
	wantErr := protovalidate.Validate(entity)
	var wantVE *protovalidate.ValidationError
	if !errors.As(wantErr, &wantVE) {
		t.Fatalf("want protovalidate.Validate(entity) to reject, got %v", wantErr)
	}

	_, gotErr := client.Policed.Create().
		SetCelField("nope").
		SetStandardField("abc").
		Save(context.Background())
	if gotErr == nil {
		t.Fatal("want an error from the ent client directly (no HTTP, no interceptor), got nil")
	}
	var gotVE *protovalidate.ValidationError
	if !errors.As(gotErr, &gotVE) {
		t.Fatalf("want a *protovalidate.ValidationError from the ent client directly, got %v (%T)", gotErr, gotErr)
	}

	want := violationIdentitySet(wantVE)
	got := violationIdentitySet(gotVE)
	if len(want) == 0 {
		t.Fatal("test bug: protovalidate.Validate(entity) produced zero violations")
	}
	for id := range want {
		if !got[id] {
			t.Errorf("storage-layer violations missing identity %q present in protovalidate.Validate(entity)'s violations", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("storage-layer violations carry unexpected identity %q not present in protovalidate.Validate(entity)'s violations", id)
		}
	}
}

// TestWiring_BoundaryAloneStillRejects is Test 3: the boundary validator,
// exercised alone (protovalidate.Validate — the identical call
// connectrpc.com/validate's own Interceptor makes internally against
// protovalidate.GlobalValidator — with NO ent.Client reached at all)
// rejects the identical input. This is the plan's prohibition made
// concrete: storage-layer enforcement is never a reason the boundary gets
// weakened — both layers stay independently armed.
func TestWiring_BoundaryAloneStillRejects(t *testing.T) {
	req := &entconnecttestv1.CreateHookWiringRequest{
		HookWiring: &entconnecttestv1.HookWiring{CelField: "nope", StandardField: "abc"},
	}
	err := protovalidate.Validate(req)
	if err == nil {
		t.Fatal("want the boundary validator to reject the request, got nil")
	}
	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want a *protovalidate.ValidationError, got %v (%T)", err, err)
	}
}

// TestWiring_ValidPayloadPersistsAndReadsBack is Test 4: a valid payload
// through the real Connect client is accepted, persisted, and readable
// back through the ent client — the positive-path counterpart to the
// three rejection proofs above.
func TestWiring_ValidPayloadPersistsAndReadsBack(t *testing.T) {
	client := newTestClient(t)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	resp, err := c.CreateHookWiring(context.Background(), connect.NewRequest(&entconnecttestv1.CreateHookWiringRequest{
		HookWiring: &entconnecttestv1.HookWiring{CelField: "Xvalid", StandardField: "abc"},
	}))
	if err != nil {
		t.Fatalf("CreateHookWiring: %v", err)
	}
	if got := resp.Msg.GetHookWiring().GetCelField(); got != "Xvalid" {
		t.Fatalf("want cel_field %q, got %q", "Xvalid", got)
	}
	if resp.Msg.GetHookWiring().GetId() == "" {
		t.Fatal("want a non-empty server-assigned id")
	}

	rows, err := client.Policed.Query().All(context.Background())
	if err != nil {
		t.Fatalf("query policed rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 persisted row, got %d", len(rows))
	}
	if rows[0].CelField != "Xvalid" {
		t.Fatalf("want persisted cel_field %q, got %q", "Xvalid", rows[0].CelField)
	}
	if rows[0].StandardField != "abc" {
		t.Fatalf("want persisted standard_field %q, got %q", "abc", rows[0].StandardField)
	}
}
