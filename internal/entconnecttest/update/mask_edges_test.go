package update

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"

	"github.com/smintz/entconnect/internal/entconnecttest/update/ent"
	"github.com/smintz/entconnect/mixinforproto"
)

// assertUnchanged re-reads id's row via client and fails t if it does
// not match want* exactly — the "mutated nothing" half of every
// rejection case in this file (D-16/D-15's whole point: a rejected
// request must never have written anything).
func assertUnchanged(t *testing.T, client *ent.Client, id int, wantTitle, wantBody string, wantRevision int32) {
	t.Helper()
	row, err := client.Patch.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("re-read row: %v", err)
	}
	if row.Title != wantTitle || row.Body != wantBody || row.Revision != wantRevision {
		t.Fatalf("want the row unchanged (title=%q body=%q revision=%d), got title=%q body=%q revision=%d",
			wantTitle, wantBody, wantRevision, row.Title, row.Body, row.Revision)
	}
}

// assertInvalidArgumentUnchanged is the shared shape for every rejection
// case below: the request must fail with CodeInvalidArgument, and the
// seeded row must be untouched afterward.
func assertInvalidArgumentUnchanged(
	t *testing.T,
	client *ent.Client,
	c entconnecttestv1connect.PatchUpdateServiceClient,
	req *entconnecttestv1.UpdatePatchRequest,
	id int,
) error {
	t.Helper()
	_, err := c.UpdatePatch(context.Background(), connect.NewRequest(req))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
	assertUnchanged(t, client, id, "original title", "original body", 1)
	return err
}

// --- Case 1: empty / absent mask (D-16) -------------------------------

func TestMaskEdges_AbsentMask(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	req := &entconnecttestv1.UpdatePatchRequest{
		Patch: &entconnecttestv1.Patch{Id: fmtInt(created.ID), Title: "should never apply"},
		// UpdateMask left entirely nil/absent.
	}
	assertInvalidArgumentUnchanged(t, client, c, req, created.ID)
}

func TestMaskEdges_ZeroLengthPaths(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	req := &entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(created.ID), Title: "should never apply"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{}},
	}
	assertInvalidArgumentUnchanged(t, client, c, req, created.ID)
}

func TestMaskEdges_EmptyStringPath(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	req := &entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(created.ID), Title: "should never apply"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{""}},
	}
	assertInvalidArgumentUnchanged(t, client, c, req, created.ID)
}

// --- Case 2: nested and wildcard paths (D-15) --------------------------

func TestMaskEdges_NestedPath(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	req := &entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(created.ID), Title: "should never apply"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"patch.title"}},
	}
	_, err := c.UpdatePatch(context.Background(), connect.NewRequest(req))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
	if got := err.Error(); !contains(got, "patch.title") {
		t.Fatalf("want the offending path %q named verbatim in the error, got: %s", "patch.title", got)
	}
	assertUnchanged(t, client, created.ID, "original title", "original body", 1)
}

func TestMaskEdges_WildcardPath(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	req := &entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(created.ID), Title: "should never apply"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"*"}},
	}
	_, err := c.UpdatePatch(context.Background(), connect.NewRequest(req))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
	if got := err.Error(); !contains(got, "*") {
		t.Fatalf("want the offending path %q named verbatim in the error, got: %s", "*", got)
	}
	assertUnchanged(t, client, created.ID, "original title", "original body", 1)
}

// --- Case 3: descriptor-unknown and excluded-field paths ---------------

func TestMaskEdges_UnknownPath(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	req := &entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(created.ID), Title: "should never apply"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"does_not_exist"}},
	}
	_, err := c.UpdatePatch(context.Background(), connect.NewRequest(req))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
	if got := err.Error(); !contains(got, "does_not_exist") {
		t.Fatalf("want the offending path %q named verbatim in the error, got: %s", "does_not_exist", got)
	}
	assertUnchanged(t, client, created.ID, "original title", "original body", 1)
}

func TestMaskEdges_ExcludedFieldPath(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	req := &entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(created.ID), InternalNote: "should never apply"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"internal_note"}},
	}
	_, err := c.UpdatePatch(context.Background(), connect.NewRequest(req))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
	if got := err.Error(); !contains(got, "internal_note") {
		t.Fatalf("want the offending path %q named verbatim in the error, got: %s", "internal_note", got)
	}
	assertUnchanged(t, client, created.ID, "original title", "original body", 1)
}

// --- Case 4: duplicates and near-equal paths ----------------------------

func TestMaskEdges_DuplicatePathAppliesOnce(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	req := connect.NewRequest(&entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(created.ID), Title: "deduped title"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title", "title"}},
	})
	resp, err := c.UpdatePatch(context.Background(), req)
	if err != nil {
		t.Fatalf("UpdatePatch: %v", err)
	}
	if got := resp.Msg.GetPatch().GetTitle(); got != "deduped title" {
		t.Fatalf("want title %q, got %q", "deduped title", got)
	}
	row, err := client.Patch.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("re-read row: %v", err)
	}
	if row.Title != "deduped title" || row.Body != "original body" || row.Revision != 1 {
		t.Fatalf("want only title updated, got title=%q body=%q revision=%d", row.Title, row.Body, row.Revision)
	}
}

func TestMaskEdges_CaseDifferingPathRejected(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	req := &entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(created.ID), Title: "should never apply"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"Title"}},
	}
	assertInvalidArgumentUnchanged(t, client, c, req, created.ID)
}

// --- Case 5: ordering -----------------------------------------------------

func TestMaskEdges_PathOrderIsIrrelevant(t *testing.T) {
	client := newTestClient(t)

	seededA, err := client.Patch.Create().SetTitle("a-title").SetBody("a-body").SetRevision(1).Save(context.Background())
	if err != nil {
		t.Fatalf("seed A: %v", err)
	}
	seededB, err := client.Patch.Create().SetTitle("a-title").SetBody("a-body").SetRevision(1).Save(context.Background())
	if err != nil {
		t.Fatalf("seed B: %v", err)
	}

	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	_, err = c.UpdatePatch(context.Background(), connect.NewRequest(&entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(seededA.ID), Title: "new title", Body: "new body", Revision: 7},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"body", "revision", "title"}},
	}))
	if err != nil {
		t.Fatalf("UpdatePatch (forward order): %v", err)
	}
	_, err = c.UpdatePatch(context.Background(), connect.NewRequest(&entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(seededB.ID), Title: "new title", Body: "new body", Revision: 7},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title", "revision", "body"}},
	}))
	if err != nil {
		t.Fatalf("UpdatePatch (reversed order): %v", err)
	}

	rowA, err := client.Patch.Get(context.Background(), seededA.ID)
	if err != nil {
		t.Fatalf("re-read A: %v", err)
	}
	rowB, err := client.Patch.Get(context.Background(), seededB.ID)
	if err != nil {
		t.Fatalf("re-read B: %v", err)
	}
	if rowA.Title != rowB.Title || rowA.Body != rowB.Body || rowA.Revision != rowB.Revision {
		t.Fatalf("want identical rows regardless of mask path order: A=%+v B=%+v", rowA, rowB)
	}
	if rowA.Title != "new title" || rowA.Body != "new body" || rowA.Revision != 7 {
		t.Fatalf("want every masked field applied, got %+v", rowA)
	}
}

// --- Case 6: the recorded length-unit divergence (or its absence) ------

// TestMaskEdges_LengthUnitDivergence answers CRUD-05's "whose definition
// of length applies here" question against Phase 1's own recorded fact
// (SourceField.LengthUnitDivergentIDs), rather than deriving a length
// rule from first principles. The Patch entity is derived here exactly
// as internal/entconnecttest/update/ent/schema/patch.go derives it
// (Exclude("id", "internal_note")) so this test inspects the SAME
// provenance the real fixture's own Update handler was generated from.
//
// The corpus's Patch message (proto/entconnecttest/v1/update.proto)
// deliberately carries no buf.validate constraints at all (see that
// file's own doc comment) — so the honest answer, per 02-04-PLAN.md Task
// 3 case 6's own sanctioned fallback, is that this corpus has no
// recorded length-unit divergence to observe. Asserting that absence
// directly (rather than inventing a divergent constraint the corpus does
// not have) IS the project's answer for this corpus.
func TestMaskEdges_LengthUnitDivergence(t *testing.T) {
	fields := mixinforproto.MixinForProto[*entconnecttestv1.Patch](
		mixinforproto.Exclude("id", "internal_note"),
	).Fields()

	if len(fields) == 0 {
		t.Fatal("want at least one derived field to inspect")
	}
	for _, f := range fields {
		for _, ann := range f.Descriptor().Annotations {
			sf, ok := ann.(mixinforproto.SourceField)
			if !ok {
				continue
			}
			if len(sf.LengthUnitDivergentIDs) != 0 {
				t.Fatalf(
					"want SourceField.LengthUnitDivergentIDs empty for field %q (this corpus records no divergence), got %v",
					sf.FieldName, sf.LengthUnitDivergentIDs,
				)
			}
		}
	}
}

// --- Case 7: idempotency at the wire -------------------------------------

func TestMaskEdges_RepeatedApplicationIsANoOp(t *testing.T) {
	client := newTestClient(t)
	created := seedPatch(t, client)
	c := newTestServer(t, client, testAuthenticator{subject: "allowed-subject"})

	req := connect.NewRequest(&entconnecttestv1.UpdatePatchRequest{
		Patch:      &entconnecttestv1.Patch{Id: fmtInt(created.ID), Title: "steady title"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title"}},
	})

	var lastTitle, lastBody string
	var lastRevision int32
	for i := 0; i < 5; i++ {
		resp, err := c.UpdatePatch(context.Background(), req)
		if err != nil {
			t.Fatalf("UpdatePatch (call %d): %v", i+1, err)
		}
		if got := resp.Msg.GetPatch().GetTitle(); got != "steady title" {
			t.Fatalf("call %d: want title %q, got %q", i+1, "steady title", got)
		}
		row, err := client.Patch.Get(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("re-read row (call %d): %v", i+1, err)
		}
		if i > 0 && (row.Title != lastTitle || row.Body != lastBody || row.Revision != lastRevision) {
			t.Fatalf("call %d: want the row unchanged from the previous call, got title=%q body=%q revision=%d (previous: title=%q body=%q revision=%d)",
				i+1, row.Title, row.Body, row.Revision, lastTitle, lastBody, lastRevision)
		}
		if row.Body != "original body" || row.Revision != 1 {
			t.Fatalf("call %d: want unmasked fields never to change, got body=%q revision=%d", i+1, row.Body, row.Revision)
		}
		lastTitle, lastBody, lastRevision = row.Title, row.Body, row.Revision
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
