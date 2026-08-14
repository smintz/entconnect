// Package list holds the List slice fixture's end-to-end proof: a real
// Connect request, over HTTP, through the generated interceptor chain,
// into a generated ListPages handler, against a real ent client backed
// by an in-memory SQLite database. No mock at any layer.
package list

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

	"github.com/smintz/entconnect/internal/entconnecttest/list/ent"
	// required by schema hooks (ent's own bootstrap convention — see the
	// identical blank import in the read fixture's own tracer_test.go).
	_ "github.com/smintz/entconnect/internal/entconnecttest/list/ent/runtime"
	"github.com/smintz/entconnect/internal/entconnecttest/list/entconnect"
	"github.com/smintz/entconnect/runtime/viewer"
)

// testViewer and testAuthenticator mirror the read fixture's own
// (internal/entconnecttest/read/tracer_test.go) — every request in this
// fixture authenticates as the same always-allowed subject, since List
// paging (not privacy) is what this fixture's tests exercise.
type testViewer struct{ subject string }

func (v testViewer) Subject() string { return v.subject }
func (v testViewer) Roles() []string { return nil }

type testAuthenticator struct{ subject string }

func (a testAuthenticator) AuthenticateAndViewer(ctx context.Context, _ http.Header) (context.Context, error) {
	return viewer.NewContext(ctx, testViewer{subject: a.subject}), nil
}

// newTestClient opens a fresh, migrated ent client backed by a unique
// in-memory SQLite database, isolated per test so tests can run with
// -race without interfering with each other.
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
// PageListService wiring, and returns a real Connect client pointed at
// it.
func newTestServer(t *testing.T, client *ent.Client) entconnecttestv1connect.PageListServiceClient {
	t.Helper()
	srv, err := entconnect.NewServer(client, testAuthenticator{subject: "allowed-subject"})
	if err != nil {
		t.Fatalf("entconnect.NewServer: %v", err)
	}
	mux := http.NewServeMux()
	srv.Register(mux)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return entconnecttestv1connect.NewPageListServiceClient(ts.Client(), ts.URL)
}

// seedPages creates n rows with strictly increasing created_at values
// (one second apart, well above SQLite's storage precision), each titled
// "page-<index>" in creation order — the harness every list_test.go test
// seeds from.
func seedPages(t *testing.T, client *ent.Client, n int) []*ent.Page {
	t.Helper()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]*ent.Page, 0, n)
	for i := 0; i < n; i++ {
		row, err := client.Page.Create().
			SetTitle(titleFor(i)).
			SetCreatedAt(base.Add(time.Duration(i) * time.Second)).
			Save(context.Background())
		if err != nil {
			t.Fatalf("seed page %d: %v", i, err)
		}
		rows = append(rows, row)
	}
	return rows
}

func titleFor(i int) string {
	return "page-" + string(rune('a'+i))
}

// TestListPages_SequentialPagesConcatenateToFullSet is the phase's
// architectural proof for CRUD-03: a first page comes back with an
// opaque next_page_token, and feeding that token to a second (then
// third) call continues exactly where the previous page stopped — no
// row skipped, no row repeated, and the final page's next_page_token is
// empty.
func TestListPages_SequentialPagesConcatenateToFullSet(t *testing.T) {
	client := newTestClient(t)
	seeded := seedPages(t, client, 7)
	c := newTestServer(t, client)

	var allIDs []string
	token := ""
	for page := 0; page < 10; page++ { // generous upper bound; loop breaks on empty token
		resp, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{
			PageSize:  3,
			PageToken: token,
		}))
		if err != nil {
			t.Fatalf("ListPages (page %d): %v", page, err)
		}
		for _, p := range resp.Msg.GetPages() {
			allIDs = append(allIDs, p.GetId())
		}
		token = resp.Msg.GetNextPageToken()
		if token == "" {
			break
		}
	}

	if len(allIDs) != len(seeded) {
		t.Fatalf("want %d total rows across all pages, got %d: %v", len(seeded), len(allIDs), allIDs)
	}
	seenSet := map[string]bool{}
	for _, id := range allIDs {
		if seenSet[id] {
			t.Fatalf("id %q returned more than once across pages: %v", id, allIDs)
		}
		seenSet[id] = true
	}
	for _, row := range seeded {
		if !seenSet[idString(row.ID)] {
			t.Fatalf("seeded id %d never appeared across any page: %v", row.ID, allIDs)
		}
	}

	// Ascending created_at order (D-20's "all ascending" requirement):
	// concatenated ids must appear in seed order.
	for i, row := range seeded {
		if allIDs[i] != idString(row.ID) {
			t.Fatalf("want ascending seed order at position %d: want id %d, got %s", i, row.ID, allIDs[i])
		}
	}
}

// TestListPages_FirstPageShape asserts the first page's shape directly:
// 3 rows in ascending order and a non-empty next_page_token when 7 rows
// exist and page_size=3.
func TestListPages_FirstPageShape(t *testing.T) {
	client := newTestClient(t)
	seeded := seedPages(t, client, 7)
	c := newTestServer(t, client)

	resp, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{PageSize: 3}))
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	got := resp.Msg.GetPages()
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d", len(got))
	}
	for i, p := range got {
		if p.GetId() != idString(seeded[i].ID) {
			t.Fatalf("position %d: want id %d, got %s", i, seeded[i].ID, p.GetId())
		}
	}
	if resp.Msg.GetNextPageToken() == "" {
		t.Fatal("want a non-empty next_page_token when more rows remain")
	}
}

// idString matches the generated handler's own "fmt.Sprint(row.ID)"
// wire-encoding of the int ent ID as a string (mirrors the read
// fixture's own fmtInt helper).
func idString(id int) string {
	return strconv.Itoa(id)
}
