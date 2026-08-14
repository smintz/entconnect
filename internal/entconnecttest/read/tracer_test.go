// Package read holds the walking-slice fixture's end-to-end proof: a
// real Connect request, over HTTP, through the generated interceptor
// chain, into a generated GetOrder handler, against a real ent client
// backed by an in-memory SQLite database. No mock at any layer.
package read

import (
	"context"
	stdsql "database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"

	"github.com/smintz/entconnect/internal/entconnecttest/read/ent"
	// required by schema hooks (ent's own bootstrap convention — see the
	// identical blank import in the generated ent/enttest package).
	_ "github.com/smintz/entconnect/internal/entconnecttest/read/ent/runtime"
	"github.com/smintz/entconnect/internal/entconnecttest/read/ent/schema"
	"github.com/smintz/entconnect/internal/entconnecttest/read/entconnect"
	"github.com/smintz/entconnect/runtime/viewer"
)

// testViewer is the tracer test's minimal viewer.Viewer implementation.
type testViewer struct {
	subject string
}

func (v testViewer) Subject() string { return v.subject }
func (v testViewer) Roles() []string { return nil }

// testAuthenticator is a test-only runtime.Authenticator. subject == ""
// means "authenticate successfully but attach no viewer" — the INT-02
// missing-viewer case; subjectErr, when non-nil, is returned as the
// authentication failure itself.
type testAuthenticator struct {
	subject   string
	attachNo  bool
	subjectFn func() string
}

func (a testAuthenticator) AuthenticateAndViewer(ctx context.Context, _ http.Header) (context.Context, error) {
	if a.attachNo {
		return ctx, nil
	}
	return viewer.NewContext(ctx, testViewer{subject: a.subject}), nil
}

// newTestClient opens a fresh, migrated ent client backed by a unique
// in-memory SQLite database (modernc.org/sqlite, CGO-free — RESEARCH.md's
// recommended test driver). Each call gets its own isolated database so
// tests can run with -race without interfering with each other.
func newTestClient(t *testing.T) *ent.Client {
	t.Helper()
	db, err := stdsql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return client
}

// newTestServer builds a real HTTP server mounting the generated
// OrderReadService wiring behind authenticator, and returns a real
// Connect client pointed at it.
func newTestServer(t *testing.T, client *ent.Client, authenticator testAuthenticator) entconnecttestv1connect.OrderReadServiceClient {
	t.Helper()
	srv, err := entconnect.NewServer(client, authenticator)
	if err != nil {
		t.Fatalf("entconnect.NewServer: %v", err)
	}
	mux := http.NewServeMux()
	srv.Register(mux)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return entconnecttestv1connect.NewOrderReadServiceClient(ts.Client(), ts.URL)
}

// TestGetOrder_RealRoundTrip is the phase's architectural proof: an
// ent-schema annotation naming a generated procedure constant resolves
// against the committed descriptor set, the entc hook's emitted handler
// runs the request through the fixed interceptor chain, and a real
// Connect client gets a real row back from a real ent client that this
// test never touches directly except to seed it.
func TestGetOrder_RealRoundTrip(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	created, err := client.Order.Create().
		SetCustomer("Acme Co").
		SetStatus("open").
		SetCreatedAt(time.Now().UTC().Truncate(time.Second)).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}

	authn := testAuthenticator{subject: "allowed-subject"}
	client2 := newTestServer(t, client, authn)

	resp, err := client2.GetOrder(context.Background(), connect.NewRequest(&entconnecttestv1.GetOrderRequest{
		Id: fmtInt(created.ID),
	}))
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if got := resp.Msg.GetOrder().GetCustomer(); got != "Acme Co" {
		t.Fatalf("want customer %q, got %q", "Acme Co", got)
	}
	if got := resp.Msg.GetOrder().GetStatus(); got != "open" {
		t.Fatalf("want status %q, got %q", "open", got)
	}
	if resp.Msg.GetOrder().GetCreatedAt() == nil {
		t.Fatal("want a non-nil created_at")
	}
}

// TestGetOrder_ProtovalidateRejectsEmptyID proves the protovalidate
// chain stage rejects an inbound request violating the contract's
// string.min_len rule (GetOrderRequest.id) before the handler ever runs.
func TestGetOrder_ProtovalidateRejectsEmptyID(t *testing.T) {
	client := newTestClient(t)
	authn := testAuthenticator{subject: "allowed-subject"}
	c := newTestServer(t, client, authn)

	_, err := c.GetOrder(context.Background(), connect.NewRequest(&entconnecttestv1.GetOrderRequest{Id: ""}))
	if err == nil {
		t.Fatal("want an error for an empty id, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
}

// TestGetOrder_MissingViewerIsUnauthenticated proves INT-02: a context
// reaching the viewer-injection stage with no viewer attached is refused
// with CodeUnauthenticated, never treated as an anonymous allow.
func TestGetOrder_MissingViewerIsUnauthenticated(t *testing.T) {
	client := newTestClient(t)
	authn := testAuthenticator{attachNo: true}
	c := newTestServer(t, client, authn)

	_, err := c.GetOrder(context.Background(), connect.NewRequest(&entconnecttestv1.GetOrderRequest{Id: "1"}))
	if err == nil {
		t.Fatal("want an error when no viewer is attached, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeUnauthenticated {
		t.Fatalf("want CodeUnauthenticated, got %v (%v)", code, err)
	}
}

// TestGetOrder_PrivacyDenyIsPermissionDenied proves INT-03: an ent
// privacy.Deny decision (schema.Order.Policy's DeniedSubject rule)
// arrives at the Connect client as CodePermissionDenied.
func TestGetOrder_PrivacyDenyIsPermissionDenied(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	created, err := client.Order.Create().
		SetCustomer("Acme Co").
		SetStatus("open").
		SetCreatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}

	authn := testAuthenticator{subject: schema.DeniedSubject}
	c := newTestServer(t, client, authn)

	_, err = c.GetOrder(context.Background(), connect.NewRequest(&entconnecttestv1.GetOrderRequest{
		Id: fmtInt(created.ID),
	}))
	if err == nil {
		t.Fatal("want an error for a denied viewer, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodePermissionDenied {
		t.Fatalf("want CodePermissionDenied, got %v (%v)", code, err)
	}
}

func fmtInt(id int) string {
	return strconv.Itoa(id)
}
