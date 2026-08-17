// Package hookwiring holds VAL-09's relative hook-ordering assertion and
// D-13's real-client wiring proof. This file is the former: a real,
// generated ent.Client proves — by a recorded marker SEQUENCE, never an
// index into any hooks slice — that a privacy Policy (when declared) runs
// before the mixin's own storage-layer validation hook, which in turn runs
// before a schema-declared hook.
//
// The property under test, in prose, matching the failure messages below:
// "privacy policy (if any) -> mixin hook -> schema-declared hooks". An ent
// upgrade that reorders defaults()/policy/mixin-hooks/schema-hooks
// (03-RESEARCH.md Summary point 4) makes one of these tests fail with a
// message naming that property and this process's entgo.io/ent version —
// not a bare slice-equality mismatch a future maintainer has to reverse-
// engineer.
package hookwiring

import (
	"context"
	stdsql "database/sql"
	"errors"
	"reflect"
	"runtime/debug"
	"testing"

	"buf.build/go/protovalidate"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/privacy"
	_ "modernc.org/sqlite"

	"github.com/smintz/entconnect/internal/entconnecttest/hookwiring/ent"
	// required by schema hooks (ent's own bootstrap convention — see the
	// identical blank import in internal/entconnecttest/{update,write}).
	_ "github.com/smintz/entconnect/internal/entconnecttest/hookwiring/ent/runtime"
	"github.com/smintz/entconnect/internal/entconnecttest/hookwiring/ent/schema"
	"github.com/smintz/entconnect/runtime/viewer"
)

// entVersion resolves the entgo.io/ent module version this test binary was
// built against, via runtime/debug.ReadBuildInfo — so a failure message
// below can name the version an ent upgrade needs to be diagnosed against,
// without hand-maintaining a version string that would itself go stale.
func entVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown (no build info)"
	}
	for _, dep := range bi.Deps {
		if dep.Path == "entgo.io/ent" {
			return dep.Version
		}
	}
	return "unknown (entgo.io/ent not found in build info)"
}

// orderingFailureMessage names the property under test and the ent
// version, so a real regression is immediately actionable rather than a
// bare slice mismatch.
const orderingProperty = "privacy policy (if any) -> mixin hook -> schema-declared hooks"

// assertMarkerSequence fails t with an actionable message (the expected
// property, the ent version, and both sequences) if got does not equal
// want exactly — a relative-sequence comparison, never an index into any
// hooks slice.
func assertMarkerSequence(t *testing.T, rec *schema.Recorder, want []string) {
	t.Helper()
	got := rec.Markers()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"marker sequence mismatch: want %v, got %v\n"+
				"expected relative order: %s\n"+
				"entgo.io/ent version: %s (an ent upgrade that reorders "+
				"defaults()/policy/mixin-hooks/schema-hooks is the first "+
				"thing to check)",
			want, got, orderingProperty, entVersion(),
		)
	}
}

// testViewer is this fixture's minimal viewer.Viewer implementation,
// mirroring internal/entconnecttest/{update,write}'s own.
type testViewer struct {
	subject string
}

func (v testViewer) Subject() string { return v.subject }
func (v testViewer) Roles() []string { return nil }

// newTestClient opens a fresh, migrated ent client backed by a unique
// in-memory SQLite database, isolated per test so tests can run with
// -race without interfering with each other — mirrors
// internal/entconnecttest/{update,write}'s own newTestClient exactly.
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

// TestOrdering_PolicyBearing_InvalidValue is Test 1: a Create whose value
// violates cel_field's residual rule records ONLY the policy marker — the
// privacy Policy ran (it always runs), then the mixin's own validation
// hook rejected the mutation before the schema-declared hook ever saw it.
func TestOrdering_PolicyBearing_InvalidValue(t *testing.T) {
	client := newTestClient(t)
	rec := &schema.Recorder{}
	ctx := schema.NewRecorderContext(context.Background(), rec)

	_, err := client.Policed.Create().
		SetCelField("nope"). // does not start with "X" -> violates the residual CEL rule
		SetStandardField("abc").
		Save(ctx)
	if err == nil {
		t.Fatal("want an error for an invalid cel_field, got nil")
	}
	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want a *protovalidate.ValidationError, got %v (%T)", err, err)
	}
	assertMarkerSequence(t, rec, []string{"policy"})
}

// TestOrdering_PolicyBearing_ValidValue is Test 2: a Create with a valid
// value records the policy marker THEN the schema-hook marker, in that
// order — the mixin's validation hook let the mutation through.
func TestOrdering_PolicyBearing_ValidValue(t *testing.T) {
	client := newTestClient(t)
	rec := &schema.Recorder{}
	ctx := schema.NewRecorderContext(context.Background(), rec)

	_, err := client.Policed.Create().
		SetCelField("Xvalid").
		SetStandardField("abc").
		Save(ctx)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	assertMarkerSequence(t, rec, []string{"policy", "schema-hook"})
}

// TestOrdering_PolicyFree_InvalidValue is Test 3: the equivalent Create on
// Unpoliced (no Policy() declared at all) records NO marker — the mixin
// hook is Hooks[0] here (no privacy wrapper is installed when no Policy()
// exists anywhere on the schema — 03-RESEARCH.md Pitfall 3) and rejects
// before the schema hook ever runs.
func TestOrdering_PolicyFree_InvalidValue(t *testing.T) {
	client := newTestClient(t)
	rec := &schema.Recorder{}
	ctx := schema.NewRecorderContext(context.Background(), rec)

	_, err := client.Unpoliced.Create().
		SetCelField("nope").
		SetStandardField("abc").
		Save(ctx)
	if err == nil {
		t.Fatal("want an error for an invalid cel_field, got nil")
	}
	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want a *protovalidate.ValidationError, got %v (%T)", err, err)
	}
	assertMarkerSequence(t, rec, []string{})
}

// TestOrdering_PolicyFree_ValidValue is Test 4: the equivalent valid Create
// on Unpoliced records ONLY the schema-hook marker.
func TestOrdering_PolicyFree_ValidValue(t *testing.T) {
	client := newTestClient(t)
	rec := &schema.Recorder{}
	ctx := schema.NewRecorderContext(context.Background(), rec)

	_, err := client.Unpoliced.Create().
		SetCelField("Xvalid").
		SetStandardField("abc").
		Save(ctx)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	assertMarkerSequence(t, rec, []string{"schema-hook"})
}

// TestOrdering_DenyingPolicy_InvalidValue is Test 5: T-03-17's mitigation.
// A denying Policy() decision on a mutation that ALSO violates cel_field's
// residual rule yields privacy.Deny, never a *protovalidate.ValidationError
// — authorization precedes validation (the combined privacy Policy is
// Hooks[0] whenever any Policy() exists), so an unauthorized caller learns
// nothing about which constraints exist on this entity.
func TestOrdering_DenyingPolicy_InvalidValue(t *testing.T) {
	client := newTestClient(t)
	rec := &schema.Recorder{}
	ctx := schema.NewRecorderContext(context.Background(), rec)
	ctx = viewer.NewContext(ctx, testViewer{subject: schema.DeniedSubject})

	_, err := client.Policed.Create().
		SetCelField("nope"). // also violates the residual CEL rule
		SetStandardField("abc").
		Save(ctx)
	if err == nil {
		t.Fatal("want an error for a denied viewer, got nil")
	}
	if !errors.Is(err, privacy.Deny) {
		t.Fatalf("want errors.Is(err, privacy.Deny) == true, got %v", err)
	}
	var ve *protovalidate.ValidationError
	if errors.As(err, &ve) {
		t.Fatalf(
			"want errors.As(err, &ve) == false (a denied caller must never "+
				"see a *protovalidate.ValidationError — that would leak "+
				"which constraints exist), got %v", ve,
		)
	}
}
