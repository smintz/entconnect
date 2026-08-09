package list

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"

	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"

	"github.com/smintz/entconnect/internal/entconnecttest/list/ent"
	entconnectruntime "github.com/smintz/entconnect/runtime"
)

// seedPagesAt seeds one row per entry in times, in the given order,
// allowing duplicate created_at values — the tie cases below need this;
// list_test.go's own seedPages only ever produces strictly increasing
// times.
func seedPagesAt(t *testing.T, client *ent.Client, times []time.Time) []*ent.Page {
	t.Helper()
	rows := make([]*ent.Page, 0, len(times))
	for i, ts := range times {
		row, err := client.Page.Create().
			SetTitle(fmt.Sprintf("edge-%d", i)).
			SetCreatedAt(ts).
			Save(context.Background())
		if err != nil {
			t.Fatalf("seed page %d: %v", i, err)
		}
		rows = append(rows, row)
	}
	return rows
}

// TestPagingEdges_TiesAtBoundary is CRUD-03's concrete proof that
// appending the primary key to the ordering tuple makes tied rows
// separate rather than collide: rows 3 and 4 share a byte-identical
// created_at value. Walked at two different page sizes so the page
// boundary falls exactly between the tied rows (page_size=2) and
// elsewhere (page_size=3) — either way the full set must come back with
// every row exactly once.
func TestPagingEdges_TiesAtBoundary(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tieTime := base.Add(2 * time.Second)
	times := []time.Time{
		base,
		base.Add(1 * time.Second),
		tieTime,
		tieTime, // rows 3 and 4 (0-indexed 2 and 3): a byte-identical created_at
		base.Add(3 * time.Second),
		base.Add(4 * time.Second),
	}

	for _, pageSize := range []int32{2, 3} {
		t.Run(fmt.Sprintf("page_size=%d", pageSize), func(t *testing.T) {
			client := newTestClient(t)
			seeded := seedPagesAt(t, client, times)
			c := newTestServer(t, client)

			var gotIDs []string
			token := ""
			for i := 0; i < 10; i++ { // generous upper bound; loop breaks on empty token
				resp, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{
					PageSize:  pageSize,
					PageToken: token,
				}))
				if err != nil {
					t.Fatalf("ListPages: %v", err)
				}
				for _, p := range resp.Msg.GetPages() {
					gotIDs = append(gotIDs, p.GetId())
				}
				token = resp.Msg.GetNextPageToken()
				if token == "" {
					break
				}
			}

			wantSet := make(map[string]bool, len(seeded))
			for _, row := range seeded {
				wantSet[idString(row.ID)] = true
			}
			gotSet := make(map[string]bool, len(gotIDs))
			for _, id := range gotIDs {
				if gotSet[id] {
					t.Fatalf("id %q returned more than once across pages: %v", id, gotIDs)
				}
				gotSet[id] = true
			}
			if len(gotSet) != len(wantSet) {
				t.Fatalf("want %d unique ids, got %d: %v", len(wantSet), len(gotSet), gotIDs)
			}
			for id := range wantSet {
				if !gotSet[id] {
					t.Fatalf("id %q missing from paged results: %v", id, gotIDs)
				}
			}
		})
	}
}

// TestPagingEdges_Empty proves an empty table returns a length-zero
// pages slice (not a nil-vs-empty ambiguity) and an empty
// next_page_token.
func TestPagingEdges_Empty(t *testing.T) {
	client := newTestClient(t)
	c := newTestServer(t, client)

	resp, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{PageSize: 3}))
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if got := len(resp.Msg.GetPages()); got != 0 {
		t.Fatalf("want 0 pages for an empty table, got %d", got)
	}
	if resp.Msg.GetNextPageToken() != "" {
		t.Fatalf("want an empty next_page_token for an empty table, got %q", resp.Msg.GetNextPageToken())
	}
}

// TestPagingEdges_SingleRow proves a single-row table returns exactly
// that row and an empty next_page_token.
func TestPagingEdges_SingleRow(t *testing.T) {
	client := newTestClient(t)
	seedPages(t, client, 1)
	c := newTestServer(t, client)

	resp, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{PageSize: 3}))
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if got := len(resp.Msg.GetPages()); got != 1 {
		t.Fatalf("want 1 page, got %d", got)
	}
	if resp.Msg.GetNextPageToken() != "" {
		t.Fatalf("want an empty next_page_token, got %q", resp.Msg.GetNextPageToken())
	}
}

// TestPagingEdges_ExactlyPageSizeRows proves the pageSize+1 overflow
// probe does not manufacture a token for a page that has no successor:
// exactly page_size rows must come back as one full page with an EMPTY
// token.
func TestPagingEdges_ExactlyPageSizeRows(t *testing.T) {
	client := newTestClient(t)
	seedPages(t, client, 3)
	c := newTestServer(t, client)

	resp, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{PageSize: 3}))
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if got := len(resp.Msg.GetPages()); got != 3 {
		t.Fatalf("want 3 pages, got %d", got)
	}
	if resp.Msg.GetNextPageToken() != "" {
		t.Fatalf("want an empty next_page_token for an exactly-full page, got %q", resp.Msg.GetNextPageToken())
	}
}

// TestPagingEdges_PageSizePlusOneRows proves the mirror case: page_size+1
// rows come back as a full first page with a NON-empty token, and the
// second page holds exactly the one remaining row.
func TestPagingEdges_PageSizePlusOneRows(t *testing.T) {
	client := newTestClient(t)
	seeded := seedPages(t, client, 4)
	c := newTestServer(t, client)

	resp, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{PageSize: 3}))
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if got := len(resp.Msg.GetPages()); got != 3 {
		t.Fatalf("want 3 pages, got %d", got)
	}
	token := resp.Msg.GetNextPageToken()
	if token == "" {
		t.Fatal("want a non-empty next_page_token when one more row remains")
	}

	resp2, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{PageSize: 3, PageToken: token}))
	if err != nil {
		t.Fatalf("ListPages (page 2): %v", err)
	}
	if got := len(resp2.Msg.GetPages()); got != 1 {
		t.Fatalf("want exactly 1 remaining row on page 2, got %d", got)
	}
	if got, want := resp2.Msg.GetPages()[0].GetId(), idString(seeded[3].ID); got != want {
		t.Fatalf("want the 4th seeded row (id %s) on page 2, got id %s", want, got)
	}
	if resp2.Msg.GetNextPageToken() != "" {
		t.Fatalf("want an empty next_page_token on the final page, got %q", resp2.Msg.GetNextPageToken())
	}
}

// TestPagingEdges_OrderingStability is the request-level counterpart of
// codegen determinism: calling ListPages with identical inputs five
// times against a dataset that contains ties must return a
// byte-identical id sequence every time.
func TestPagingEdges_OrderingStability(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tieTime := base.Add(1 * time.Second)
	times := []time.Time{base, tieTime, tieTime, base.Add(2 * time.Second)}

	client := newTestClient(t)
	seedPagesAt(t, client, times)
	c := newTestServer(t, client)

	var first []string
	for i := 0; i < 5; i++ {
		resp, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{PageSize: 10}))
		if err != nil {
			t.Fatalf("ListPages (call %d): %v", i, err)
		}
		var ids []string
		for _, p := range resp.Msg.GetPages() {
			ids = append(ids, p.GetId())
		}
		if i == 0 {
			first = ids
			continue
		}
		if len(ids) != len(first) {
			t.Fatalf("call %d: want %d ids, got %d (want %v, got %v)", i, len(first), len(ids), first, ids)
		}
		for j := range ids {
			if ids[j] != first[j] {
				t.Fatalf("call %d: order differs at position %d: want %v, got %v", i, j, first, ids)
			}
		}
	}
}

// TestPagingEdges_FingerprintMismatch proves D-08: a token whose
// embedded fingerprint was minted against a different ordering-field set
// than the fixture's own (CreatedAt, ID) tuple fails with
// CodeInvalidArgument, never a silently inconsistent page and never
// CodeInternal.
func TestPagingEdges_FingerprintMismatch(t *testing.T) {
	client := newTestClient(t)
	seedPages(t, client, 3)
	c := newTestServer(t, client)

	badToken := entconnectruntime.EncodeCursor(entconnectruntime.Fingerprint("WrongOrderingField"), "x")

	_, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{
		PageSize:  2,
		PageToken: badToken,
	}))
	if err == nil {
		t.Fatal("want an error for a fingerprint-mismatched token, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
}

// TestPagingEdges_MalformedToken proves a syntactically invalid token
// (not base64) fails with the same CodeInvalidArgument as a fingerprint
// mismatch.
func TestPagingEdges_MalformedToken(t *testing.T) {
	client := newTestClient(t)
	seedPages(t, client, 3)
	c := newTestServer(t, client)

	_, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{
		PageSize:  2,
		PageToken: "not-valid-base64!!!###",
	}))
	if err == nil {
		t.Fatal("want an error for a malformed token, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
}

// TestPagingEdges_PageSizeClampingDefault proves page_size=0 yields
// exactly the documented default page size (50), not merely "some"
// default: 55 seeded rows with page_size=0 must return exactly 50 rows
// and a non-empty token — a default other than exactly 50 would fail
// this assertion in either direction.
func TestPagingEdges_PageSizeClampingDefault(t *testing.T) {
	client := newTestClient(t)
	seedPages(t, client, 55)
	c := newTestServer(t, client)

	resp, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{PageSize: 0}))
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if got := len(resp.Msg.GetPages()); got != 50 {
		t.Fatalf("want the documented default page size (50) when page_size=0, got %d", got)
	}
	if resp.Msg.GetNextPageToken() == "" {
		t.Fatal("want a non-empty next_page_token: 55 seeded rows, 50 returned")
	}
}

// TestPagingEdges_PageSizeClampingMax proves a page_size far above the
// upper bound is clamped to exactly the documented maximum (100), never
// an unbounded query (T-02-13): 110 seeded rows with an oversized
// page_size must return exactly 100 rows, not all 110.
func TestPagingEdges_PageSizeClampingMax(t *testing.T) {
	client := newTestClient(t)
	seedPages(t, client, 110)
	c := newTestServer(t, client)

	resp, err := c.ListPages(context.Background(), connect.NewRequest(&entconnecttestv1.ListPagesRequest{PageSize: 1_000_000}))
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if got := len(resp.Msg.GetPages()); got != 100 {
		t.Fatalf("want the documented max page size (100) even when page_size is far above it, got %d", got)
	}
	if resp.Msg.GetNextPageToken() == "" {
		t.Fatal("want a non-empty next_page_token: 110 seeded rows, 100 returned (clamped)")
	}
}
