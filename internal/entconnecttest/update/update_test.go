// Package update holds the FieldMask-gated update fixture's end-to-end
// proof: a real Connect request, over HTTP, through the generated
// interceptor chain, into a generated UpdatePatch handler, against a
// real ent client backed by an in-memory SQLite database. No mock at any
// layer. Mirrors internal/entconnecttest/read/tracer_test.go's shape
// (02-01).
package update

import (
	"context"
	stdsql "database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	_ "modernc.org/sqlite"

	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"

	"github.com/smintz/entconnect/internal/entconnecttest/update/ent"
	// required by schema hooks (ent's own bootstrap convention — see the
	// identical blank import in the generated ent/enttest package).
	_ "github.com/smintz/entconnect/internal/entconnecttest/update/ent/runtime"
	"github.com/smintz/entconnect/internal/entconnecttest/update/ent/schema"
	"github.com/smintz/entconnect/internal/entconnecttest/update/entconnect"
	"github.com/smintz/entconnect/runtime/viewer"
)

// testViewer is the fixture's minimal viewer.Viewer implementation.
type testViewer struct {
	subject string
}

func (v testViewer) Subject() string { return v.subject }
func (v testViewer) Roles() []string { return nil }

// testAuthenticator is a test-only runtime.Authenticator.
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
// in-memory SQLite database (modernc.org/sqlite, CGO-free), isolated per
// test so tests can run with -race without interfering with each other.
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
// PatchUpdateService wiring behind authenticator, and returns a real
// Connect client pointed at it.
func newTestServer(t *testing.T, client *ent.Client, authenticator testAuthenticator) entconnecttestv1connect.PatchUpdateServiceClient {
	t.Helper()
	srv, err := entconnect.NewServer(client, authenticator)
	if err != nil {
		t.Fatalf("entconnect.NewServer: %v", err)
	}
	mux := http.NewServeMux()
	srv.Register(mux)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return entconnecttestv1connect.NewPatchUpdateServiceClient(ts.Client(), ts.URL)
}

func fmtInt(id int) string {
	return strconv.Itoa(id)
}

func seedPatch(t *testing.T, client *ent.Client) *ent.Patch {
	t.Helper()
	created, err := client.Patch.Create().
		SetTitle("original title").
		SetBody("original body").
		SetRevision(1).
		Save(context.Background())
	if err != nil {
		t.Fatalf("seed patch: %v", err)
	}
	return created
}

// TestUpdate_MaskedFieldChangesUnmaskedFieldsSurvive is the plan's
// architectural proof and the point of the whole task: a masked Update
// applies exactly the masked Set calls, and a field left at its
// request's proto3 Go zero value but ABSENT from the mask keeps its
// stored value — never silently zeroed.
func TestUpdate_MaskedFieldChangesUnmaskedFieldsSurvive(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)

	authn := testAuthenticator{subject: "allowed-subject"}
	c := newTestServer(t, client, authn)

	// The request's Patch carries a new title and leaves body/revision
	// at their Go zero values ("", 0) — update_mask names only "title".
	req := connect.NewRequest(&entconnecttestv1.UpdatePatchRequest{
		Patch: &entconnecttestv1.Patch{
			Id:    fmtInt(created.ID),
			Title: "new title",
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title"}},
	})

	resp, err := c.UpdatePatch(context.Background(), req)
	if err != nil {
		t.Fatalf("UpdatePatch: %v", err)
	}
	if got := resp.Msg.GetPatch().GetTitle(); got != "new title" {
		t.Fatalf("want title %q, got %q", "new title", got)
	}

	row, err := client.Patch.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("re-read row: %v", err)
	}
	if row.Title != "new title" {
		t.Fatalf("want stored title %q, got %q", "new title", row.Title)
	}
	// The zero-collapse assertion: body and revision were NOT in the
	// mask, and the request's Patch left them at their Go zero values
	// ("" and 0) — they must retain their originally-seeded values.
	if row.Body != "original body" {
		t.Fatalf("want unmasked body to survive unchanged, got %q", row.Body)
	}
	if row.Revision != 1 {
		t.Fatalf("want unmasked revision to survive unchanged, got %d", row.Revision)
	}

	// Idempotency: issue the identical masked request a second time and
	// assert the row is byte-identical to after the first call.
	resp2, err := c.UpdatePatch(context.Background(), req)
	if err != nil {
		t.Fatalf("UpdatePatch (repeat): %v", err)
	}
	if got := resp2.Msg.GetPatch().GetTitle(); got != "new title" {
		t.Fatalf("repeat: want title %q, got %q", "new title", got)
	}
	row2, err := client.Patch.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("re-read row after repeat: %v", err)
	}
	if row2.Title != row.Title || row2.Body != row.Body || row2.Revision != row.Revision {
		t.Fatalf("want the row unchanged by a repeated identical request: first=%+v second=%+v", row, row2)
	}
}

// TestUpdate_PrivacyDenyIsPermissionDenied proves T-02-22: an ent
// privacy.Deny decision on the Update mutation (schema.Patch.Policy's
// DeniedSubject rule) arrives at the Connect client as
// CodePermissionDenied.
func TestUpdate_PrivacyDenyIsPermissionDenied(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)

	authn := testAuthenticator{subject: schema.DeniedSubject}
	c := newTestServer(t, client, authn)

	req := connect.NewRequest(&entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(created.ID), Title: "denied title"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title"}},
	})

	_, err := c.UpdatePatch(context.Background(), req)
	if err == nil {
		t.Fatal("want an error for a denied viewer, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodePermissionDenied {
		t.Fatalf("want CodePermissionDenied, got %v (%v)", code, err)
	}

	row, err := client.Patch.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("re-read row: %v", err)
	}
	if row.Title != "original title" {
		t.Fatalf("want the denied mutation to leave the row untouched, got title %q", row.Title)
	}
}
