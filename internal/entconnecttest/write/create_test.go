// Package write holds the write-slice fixture's end-to-end proof: real
// Connect requests, over HTTP, through the generated interceptor chain,
// into generated Create/Delete handlers, against a real ent client
// backed by an in-memory SQLite database. No mock at any layer — mirrors
// internal/entconnecttest/read/tracer_test.go's harness shape exactly.
package write

import (
	"context"
	stdsql "database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"

	"github.com/smintz/entconnect/internal/entconnecttest/write/ent"
	// required by schema hooks (ent's own bootstrap convention — see the
	// identical blank import in the generated ent/enttest package).
	_ "github.com/smintz/entconnect/internal/entconnecttest/write/ent/runtime"
	"github.com/smintz/entconnect/internal/entconnecttest/write/ent/schema"
	"github.com/smintz/entconnect/internal/entconnecttest/write/entconnect"
	"github.com/smintz/entconnect/runtime/viewer"
)

// testViewer is the shared harness's minimal viewer.Viewer implementation.
type testViewer struct {
	subject string
}

func (v testViewer) Subject() string { return v.subject }
func (v testViewer) Roles() []string { return nil }

// testAuthenticator is a test-only runtime.Authenticator. subject == ""
// with attachNo set means "authenticate successfully but attach no
// viewer".
type testAuthenticator struct {
	subject  string
	attachNo bool
}

func (a testAuthenticator) AuthenticateAndViewer(ctx context.Context, _ http.Header) (context.Context, error) {
	if a.attachNo {
		return ctx, nil
	}
	return viewer.NewContext(ctx, testViewer{subject: a.subject}), nil
}

// newTestClient opens a fresh, migrated ent client backed by a unique
// in-memory SQLite database (modernc.org/sqlite, CGO-free). Each call
// gets its own isolated database so tests can run with -race without
// interfering with each other.
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
// entconnect.NewServer wiring (covering every RPC bound on this fixture's
// Item schema) behind authenticator, and returns real Connect clients for
// both services pointed at it.
func newTestServer(t *testing.T, client *ent.Client, authenticator testAuthenticator) (entconnecttestv1connect.ItemCreateServiceClient, entconnecttestv1connect.ItemDeleteServiceClient) {
	t.Helper()
	srv, err := entconnect.NewServer(client, authenticator)
	if err != nil {
		t.Fatalf("entconnect.NewServer: %v", err)
	}
	mux := http.NewServeMux()
	srv.Register(mux)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return entconnecttestv1connect.NewItemCreateServiceClient(ts.Client(), ts.URL),
		entconnecttestv1connect.NewItemDeleteServiceClient(ts.Client(), ts.URL)
}

// TestCreateItem_RealRoundTrip is the write slice's architectural proof
// for Create: a real Connect client persists a real row through a
// generated handler, inside the fixed interceptor chain, against a real
// ent client this test never touches directly except to verify.
func TestCreateItem_RealRoundTrip(t *testing.T) {
	client := newTestClient(t)
	authn := testAuthenticator{subject: "allowed-subject"}
	c, _ := newTestServer(t, client, authn)

	resp, err := c.CreateItem(context.Background(), connect.NewRequest(&entconnecttestv1.CreateItemRequest{
		Item: &entconnecttestv1.Item{Sku: "SKU-1", Name: "Widget", Quantity: 10},
	}))
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if got := resp.Msg.GetItem().GetSku(); got != "SKU-1" {
		t.Fatalf("want sku %q, got %q", "SKU-1", got)
	}
	if got := resp.Msg.GetItem().GetName(); got != "Widget" {
		t.Fatalf("want name %q, got %q", "Widget", got)
	}
	if got := resp.Msg.GetItem().GetQuantity(); got != 10 {
		t.Fatalf("want quantity %d, got %d", 10, got)
	}
	if resp.Msg.GetItem().GetId() == "" {
		t.Fatal("want a non-empty server-assigned id")
	}

	// Assert the row is actually queryable through the ent client
	// afterwards — not just echoed back in the response.
	rows, err := client.Item.Query().All(context.Background())
	if err != nil {
		t.Fatalf("query items: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 persisted row, got %d", len(rows))
	}
	if rows[0].Sku != "SKU-1" {
		t.Fatalf("want persisted sku %q, got %q", "SKU-1", rows[0].Sku)
	}
}

// TestCreateItem_DuplicateSkuIsAlreadyExists proves CRUD-02's idempotency
// edge: creating the same uniquely-constrained sku twice returns
// CodeAlreadyExists on the second call, not a duplicate row and not
// CodeInternal.
func TestCreateItem_DuplicateSkuIsAlreadyExists(t *testing.T) {
	client := newTestClient(t)
	authn := testAuthenticator{subject: "allowed-subject"}
	c, _ := newTestServer(t, client, authn)

	req := connect.NewRequest(&entconnecttestv1.CreateItemRequest{
		Item: &entconnecttestv1.Item{Sku: "DUPLICATE-SKU", Name: "First", Quantity: 1},
	})
	if _, err := c.CreateItem(context.Background(), req); err != nil {
		t.Fatalf("first CreateItem: %v", err)
	}

	req2 := connect.NewRequest(&entconnecttestv1.CreateItemRequest{
		Item: &entconnecttestv1.Item{Sku: "DUPLICATE-SKU", Name: "Second", Quantity: 2},
	})
	_, err := c.CreateItem(context.Background(), req2)
	if err == nil {
		t.Fatal("want an error for a duplicate sku, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeAlreadyExists {
		t.Fatalf("want CodeAlreadyExists, got %v (%v)", code, err)
	}

	rows, err := client.Item.Query().All(context.Background())
	if err != nil {
		t.Fatalf("query items: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 persisted row after the duplicate rejection, got %d", len(rows))
	}
}

// TestCreateItem_ProtovalidateRejectsEmptySku proves the protovalidate
// chain stage rejects an inbound request violating the contract's
// string.min_len rule (Item.sku) before the handler ever runs.
func TestCreateItem_ProtovalidateRejectsEmptySku(t *testing.T) {
	client := newTestClient(t)
	authn := testAuthenticator{subject: "allowed-subject"}
	c, _ := newTestServer(t, client, authn)

	_, err := c.CreateItem(context.Background(), connect.NewRequest(&entconnecttestv1.CreateItemRequest{
		Item: &entconnecttestv1.Item{Sku: "", Name: "Widget", Quantity: 1},
	}))
	if err == nil {
		t.Fatal("want an error for an empty sku, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
}

// TestCreateItem_PrivacyDenyIsPermissionDenied proves T-02-09: a real ent
// privacy.Deny decision on a mutation (schema.Item.Policy's DeniedSubject
// rule) arrives at the Connect client as CodePermissionDenied.
func TestCreateItem_PrivacyDenyIsPermissionDenied(t *testing.T) {
	client := newTestClient(t)
	authn := testAuthenticator{subject: schema.DeniedSubject}
	c, _ := newTestServer(t, client, authn)

	_, err := c.CreateItem(context.Background(), connect.NewRequest(&entconnecttestv1.CreateItemRequest{
		Item: &entconnecttestv1.Item{Sku: "SKU-DENIED", Name: "Widget", Quantity: 1},
	}))
	if err == nil {
		t.Fatal("want an error for a denied viewer, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodePermissionDenied {
		t.Fatalf("want CodePermissionDenied, got %v (%v)", code, err)
	}

	rows, err := client.Item.Query().All(context.Background())
	if err != nil {
		t.Fatalf("query items: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("want 0 persisted rows after a denied create, got %d", len(rows))
	}
}
