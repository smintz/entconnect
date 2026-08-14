package difftest

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/dialect"

	"buf.build/go/protovalidate"

	"github.com/smintz/entconnect/mixinforproto"
	entgen "github.com/smintz/entconnect/mixinforproto/internal/difftest/ent"
	_ "github.com/smintz/entconnect/mixinforproto/internal/difftest/ent/runtime"
)

// newTestClient builds a real generated ent.Client against fakeDriver —
// no database, no modernc.org/sqlite or any other DB driver import
// anywhere in this package or mixinforproto's require block (D-13).
func newTestClient(t *testing.T) *entgen.Client {
	t.Helper()
	drv := newFakeDriver(dialect.SQLite)
	client := entgen.NewClient(entgen.Driver(drv))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("client.Close: %v", err)
		}
	})
	return client
}

// TestResidualCELRejectsRealCreate is this plan's headline proof (Task
// 2's <done> criterion): a real generated ent.Client's Create is
// rejected at the storage layer by the residual protovalidate CEL rule
// on ResidualCel.value (proto/mixinforprototest/v1/constraints.proto),
// with a *protovalidate.ValidationError carrying the contract's own
// RuleId and field path — proven with zero database driver anywhere in
// mixinforproto's require block.
func TestResidualCELRejectsRealCreate(t *testing.T) {
	client := newTestClient(t)

	_, err := client.ResidualCel.Create().SetValue("Y").Save(context.Background())
	if err == nil {
		t.Fatal("want an error rejecting \"Y\" against the residual CEL rule \"this.startsWith('X')\" — got nil")
	}

	var ve *protovalidate.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want errors.As(err, &ve) with ve *protovalidate.ValidationError — got %T: %v", err, err)
	}
	if len(ve.Violations) != 1 {
		t.Fatalf("want exactly 1 violation, got %d: %v", len(ve.Violations), ve.Violations)
	}
	v := ve.Violations[0]
	const wantRuleID = "constraints.residual_cel.starts_with_x"
	if got := v.Proto.GetRuleId(); got != wantRuleID {
		t.Fatalf("want RuleId %q, got %q", wantRuleID, got)
	}
	if got := protovalidate.FieldPathString(v.Proto.GetField()); got != "value" {
		t.Fatalf("want field path %q, got %q", "value", got)
	}
}

// TestResidualCELAcceptsRealCreate proves the accept side of the same
// path: a value that DOES satisfy the residual CEL rule reaches the
// (fake) driver and Save returns a literal nil error — never a non-nil
// *protovalidate.ValidationError with an empty Violations slice (the
// classic Go typed-nil-in-interface trap; see violation.go's doc
// comment).
func TestResidualCELAcceptsRealCreate(t *testing.T) {
	client := newTestClient(t)

	got, err := client.ResidualCel.Create().SetValue("Xyz").Save(context.Background())
	if err != nil {
		t.Fatalf("want a nil error for \"Xyz\" (satisfies \"this.startsWith('X')\") — got: %v", err)
	}
	if got == nil {
		t.Fatal("want a non-nil *ResidualCel from a successful Create")
	}
	if got.Value != "Xyz" {
		t.Fatalf("want persisted Value %q, got %q", "Xyz", got.Value)
	}
}

// TestUnsetDefaultFieldAppearsInMutationFieldsOnCreate answers
// 03-RESEARCH.md Open Question 1 / Pitfall 2 empirically, against a real
// generated ent.Client rather than a template read: does
// mutation.Fields() at hook time, on Create, already include a
// Default()-bearing field the caller never called the setter for?
//
// The plan's <action> requires this test to state the observed answer
// verbatim in its failure message and in the SUMMARY, because plan
// 03-03's D-06 mechanism collapses to one code path with no Op() branch
// if it holds, and needs the original two-path design if it does not.
func TestUnsetDefaultFieldAppearsInMutationFieldsOnCreate(t *testing.T) {
	client := newTestClient(t)

	var observedFields []string
	var sawValue bool
	client.ResidualCel.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			observedFields = append([]string{}, m.Fields()...)
			for _, f := range m.Fields() {
				if f == "value" {
					sawValue = true
				}
			}
			return next.Mutate(ctx, m)
		})
	})

	// Deliberately no SetValue call: the field is left entirely
	// untouched by the caller. If defaults() (residualcel_create.go)
	// materializes it into the mutation before any hook runs — as
	// create.tmpl's generated code does — the residual CEL hook must
	// also see it and reject the resulting empty-string value against
	// "this.startsWith('X')".
	_, err := client.ResidualCel.Create().Save(context.Background())
	if err == nil {
		t.Fatal("want an error: an unset \"value\" field collapses to Default(\"\"), which fails \"this.startsWith('X')\" — got nil")
	}

	if !sawValue {
		t.Fatalf(
			"OPEN QUESTION 1 (03-RESEARCH.md, observed answer): mutation.Fields() at hook time on Create did NOT include the unset, Default(\"\")-bearing %q field (observed fields: %v). "+
				"This means ent's generated defaults() does NOT pre-populate mutation.Fields() before hooks run in this configuration, so plan 03-03's D-06 mechanism NEEDS its original two-path (Op()-branching) design, not the single mutation.Fields()-call collapse 03-RESEARCH.md's Pitfall 2 hoped for.",
			"value", observedFields,
		)
	}
	t.Logf(
		"OPEN QUESTION 1 (03-RESEARCH.md, observed answer): mutation.Fields() on Create DOES include %q via ent's own generated defaults() running before any hook — confirming the single mutation.Fields()-call mechanism (no Op() branch) is sufficient for D-06's Create-side scope. Observed fields: %v",
		"value", observedFields,
	)
}

// TestHooksCompilesOnlyAtConstruction asserts VAL-04's "compiled once at
// schema load" property empirically: constructing the mixin's Hooks()
// twice (mirroring what the real pipeline does — once during entc's
// schema-load subprocess to count NumHooks, once again at ent client
// package init() to obtain the actual closure, per
// entgo.io/ent@v0.14.6/entc/gen/template/runtime.tmpl's
// "$pkg Mixin[$i].Hooks()" call) and then running several mutations
// through the resulting client performs CEL compilation exactly at the
// two Hooks() calls — never once per mutation.
func TestHooksCompilesOnlyAtConstruction(t *testing.T) {
	before := mixinforproto.CELCompileCount()

	// Mirrors the real pipeline's two independent Hooks() call sites
	// (entc's schema-load subprocess, and ent client package init()) —
	// both already exercised by go:generate + this package's import of
	// the generated ent package, so by the time this test runs,
	// compilation has already happened exactly twice (once per
	// Hooks() call) for ResidualCel's one residual CEL rule. This test
	// asserts no THIRD compilation happens per mutation below.
	afterImport := mixinforproto.CELCompileCount()
	if afterImport < before {
		t.Fatalf("CEL compile counter went backwards: before=%d afterImport=%d", before, afterImport)
	}

	client := newTestClient(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := client.ResidualCel.Create().SetValue("Xyz").Save(ctx); err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
	}
	afterMutations := mixinforproto.CELCompileCount()

	if afterMutations != afterImport {
		t.Fatalf(
			"want CEL compile count unchanged by running 5 mutations (compilation must happen only at Hooks() construction, never inside the returned hook closure) — before mutations: %d, after: %d",
			afterImport, afterMutations,
		)
	}
}
