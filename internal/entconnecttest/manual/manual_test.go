// Package manual holds the Manual-escape-hatch fixture's end-to-end
// proof: a real Connect request, over HTTP, through the generated
// interceptor chain, into a HAND-WRITTEN ArchiveAdmin handler body the
// application itself supplies — never generated, never touching the ent
// client — proving INT-04: the hand-written body runs inside the identical
// authn -> viewer injection -> protovalidate -> otel chain the CRUD
// handlers use. Mirrors internal/entconnecttest/read/tracer_test.go's
// harness shape.
package manual

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

	"github.com/smintz/entconnect/internal/entconnecttest/manual/ent"
	// required by schema hooks (ent's own bootstrap convention — see the
	// identical blank import in the generated ent/enttest package).
	_ "github.com/smintz/entconnect/internal/entconnecttest/manual/ent/runtime"
	"github.com/smintz/entconnect/internal/entconnecttest/manual/entconnect"
	"github.com/smintz/entconnect/runtime/viewer"
)

// testViewer is the shared harness's minimal viewer.Viewer implementation.
type testViewer struct {
	subject string
}

func (v testViewer) Subject() string { return v.subject }
func (v testViewer) Roles() []string { return nil }

// testAuthenticator is a test-only runtime.Authenticator. attachNo set
// means "authenticate successfully but attach no viewer" (the INT-02
// missing-viewer case).
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
// interfering with each other. The manual handler never touches this
// client — it exists only because NewServer's signature requires one.
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

// unimplementedPing is supplied to every test server so NewServer's
// compile-time obligation for the unclaimed Ping method (Task 1/T-02-29)
// is always satisfied — no test in this file exercises Ping itself.
func unimplementedPing(_ context.Context, _ *connect.Request[entconnecttestv1.PingRequest]) (*connect.Response[entconnecttestv1.PingResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

// newTestServer builds a real HTTP server mounting the generated
// AdminService wiring behind authenticator, with archiveAdmin as the
// hand-written ArchiveAdmin body, and returns a real Connect client
// pointed at it.
func newTestServer(t *testing.T, client *ent.Client, authenticator testAuthenticator, archiveAdmin func(context.Context, *connect.Request[entconnecttestv1.ArchiveAdminRequest]) (*connect.Response[entconnecttestv1.ArchiveAdminResponse], error)) entconnecttestv1connect.AdminServiceClient {
	t.Helper()
	srv, err := entconnect.NewServer(client, authenticator, archiveAdmin, unimplementedPing)
	if err != nil {
		t.Fatalf("entconnect.NewServer: %v", err)
	}
	mux := http.NewServeMux()
	srv.Register(mux)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return entconnecttestv1connect.NewAdminServiceClient(ts.Client(), ts.URL)
}

// TestManual_HandwrittenBodyRuns proves INT-04's core claim: the
// generated wiring for a Manual binding calls the application-supplied
// func, unmodified — the hand-written body, not any generated ent-client
// code, produces the response.
func TestManual_HandwrittenBodyRuns(t *testing.T) {
	client := newTestClient(t)

	var bodyRan bool
	archiveAdmin := func(_ context.Context, req *connect.Request[entconnecttestv1.ArchiveAdminRequest]) (*connect.Response[entconnecttestv1.ArchiveAdminResponse], error) {
		bodyRan = true
		return connect.NewResponse(&entconnecttestv1.ArchiveAdminResponse{Archived: true}), nil
	}

	authn := testAuthenticator{subject: "allowed-subject"}
	c := newTestServer(t, client, authn, archiveAdmin)

	resp, err := c.ArchiveAdmin(context.Background(), connect.NewRequest(&entconnecttestv1.ArchiveAdminRequest{Id: "1"}))
	if err != nil {
		t.Fatalf("ArchiveAdmin: %v", err)
	}
	if !bodyRan {
		t.Fatal("want the hand-written body to have run, it did not")
	}
	if !resp.Msg.GetArchived() {
		t.Fatal("want archived=true from the hand-written body's own response")
	}
}

// TestManual_MissingViewerIsUnauthenticated proves T-02-24/INT-02: a
// Manual method is a member of the SAME generated <Service>Handler
// interface as every CRUD method, so it inherits the identical chain —
// a missing viewer is refused with CodeUnauthenticated BEFORE the
// hand-written body ever runs, never treated as an anonymous allow.
func TestManual_MissingViewerIsUnauthenticated(t *testing.T) {
	client := newTestClient(t)

	var bodyRan bool
	archiveAdmin := func(_ context.Context, req *connect.Request[entconnecttestv1.ArchiveAdminRequest]) (*connect.Response[entconnecttestv1.ArchiveAdminResponse], error) {
		bodyRan = true
		return connect.NewResponse(&entconnecttestv1.ArchiveAdminResponse{Archived: true}), nil
	}

	authn := testAuthenticator{attachNo: true}
	c := newTestServer(t, client, authn, archiveAdmin)

	_, err := c.ArchiveAdmin(context.Background(), connect.NewRequest(&entconnecttestv1.ArchiveAdminRequest{Id: "1"}))
	if err == nil {
		t.Fatal("want an error when no viewer is attached, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeUnauthenticated {
		t.Fatalf("want CodeUnauthenticated, got %v (%v)", code, err)
	}
	if bodyRan {
		t.Fatal("want the hand-written body NOT to have run when the viewer is missing")
	}
}

// TestManual_ProtovalidateRejectsEmptyID proves T-02-24: a request
// violating ArchiveAdminRequest's contract buf.validate rule
// (string.min_len=1 on id) is refused with CodeInvalidArgument BEFORE
// the hand-written body ever runs — the same protovalidate chain stage
// every CRUD handler shares.
func TestManual_ProtovalidateRejectsEmptyID(t *testing.T) {
	client := newTestClient(t)

	var bodyRan bool
	archiveAdmin := func(_ context.Context, req *connect.Request[entconnecttestv1.ArchiveAdminRequest]) (*connect.Response[entconnecttestv1.ArchiveAdminResponse], error) {
		bodyRan = true
		return connect.NewResponse(&entconnecttestv1.ArchiveAdminResponse{Archived: true}), nil
	}

	authn := testAuthenticator{subject: "allowed-subject"}
	c := newTestServer(t, client, authn, archiveAdmin)

	_, err := c.ArchiveAdmin(context.Background(), connect.NewRequest(&entconnecttestv1.ArchiveAdminRequest{Id: ""}))
	if err == nil {
		t.Fatal("want an error for an empty id, got nil")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (%v)", code, err)
	}
	if bodyRan {
		t.Fatal("want the hand-written body NOT to have run for an invalid request")
	}
}

// TestManual_ViewerReadableInsideHandler proves the viewer the
// authenticator injected is readable from the context inside the
// hand-written body via viewer.FromContext — the hand-written handler
// gets the same context every generated CRUD handler gets, not a
// stripped-down one.
func TestManual_ViewerReadableInsideHandler(t *testing.T) {
	client := newTestClient(t)

	var gotSubject string
	var gotOK bool
	archiveAdmin := func(ctx context.Context, req *connect.Request[entconnecttestv1.ArchiveAdminRequest]) (*connect.Response[entconnecttestv1.ArchiveAdminResponse], error) {
		v, ok := viewer.FromContext(ctx)
		gotOK = ok
		if ok {
			gotSubject = v.Subject()
		}
		return connect.NewResponse(&entconnecttestv1.ArchiveAdminResponse{Archived: true}), nil
	}

	authn := testAuthenticator{subject: "allowed-subject"}
	c := newTestServer(t, client, authn, archiveAdmin)

	if _, err := c.ArchiveAdmin(context.Background(), connect.NewRequest(&entconnecttestv1.ArchiveAdminRequest{Id: "1"})); err != nil {
		t.Fatalf("ArchiveAdmin: %v", err)
	}
	if !gotOK {
		t.Fatal("want viewer.FromContext to succeed inside the hand-written body")
	}
	if gotSubject != "allowed-subject" {
		t.Fatalf("want subject %q, got %q", "allowed-subject", gotSubject)
	}
}
