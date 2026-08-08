package mixinforproto

import (
	"sort"
	"strings"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/proto"

	"buf.build/go/protovalidate"

	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// rawFieldByName returns the raw ent.Field named n from d, or nil. Unlike
// fieldByName (fieldmap_test.go), this exposes the real Descriptor() so a
// test can invoke the actual validator closures rather than only reading
// the JSON-safe fieldproj projection's ValidatorCount.
func rawFieldByName(d *derivation, n string) ent.Field {
	for _, f := range d.fields {
		if f.Descriptor().Name == n {
			return f
		}
	}
	return nil
}

// validatorsOf returns f's validators typed as func(T) error, ignoring
// any that do not assert to T (there should never be more than one
// numeric/string validator per field in this corpus — each Tier 1 rule
// class contributes exactly one closure).
func validatorsOf[T any](f ent.Field) []func(T) error {
	if f == nil {
		return nil
	}
	d := f.Descriptor()
	out := make([]func(T) error, 0, len(d.Validators))
	for _, v := range d.Validators {
		if fn, ok := v.(func(T) error); ok {
			out = append(out, fn)
		}
	}
	return out
}

// mustDerive derives M and fails the test immediately on error.
func mustDerive[M proto.Message](t *testing.T) *derivation {
	t.Helper()
	d, err := derive[M]()
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	return d
}

// --- VAL-01: string translation ------------------------------------------

func TestNoRulesFieldIsClean(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.NoRules](t)
	f := fieldByName(d, "plain")
	if f == nil {
		t.Fatal("want a derived field named plain")
	}
	if f.ValidatorCount != 0 {
		t.Fatalf("want 0 validators for a constraint-free field, got %d", f.ValidatorCount)
	}
	if f.SourceField == nil {
		t.Fatal("want a SourceField annotation")
	}
	if len(f.SourceField.TranslatedIDs) != 0 || len(f.SourceField.ResidualIDs) != 0 {
		t.Fatalf("want empty TranslatedIDs/ResidualIDs, got %+v", f.SourceField)
	}
}

func TestTier1StringByteBounds(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.StringByteBounds](t)

	cases := []struct {
		field, id string
		accept    string
		reject    string
	}{
		{"min_bytes_field", "string.min_bytes", "ab", "a"},
		{"max_bytes_field", "string.max_bytes", "12345678", "123456789"},
	}
	for _, c := range cases {
		f := fieldByName(d, c.field)
		if f == nil || f.SourceField == nil {
			t.Fatalf("%s: want a derived field with a SourceField annotation", c.field)
		}
		if !contains(f.SourceField.TranslatedIDs, c.id) {
			t.Fatalf("%s: want %q in TranslatedIDs, got %v", c.field, c.id, f.SourceField.TranslatedIDs)
		}
		if len(f.SourceField.LengthUnitDivergentIDs) != 0 {
			t.Fatalf("%s: byte-semantic bounds must never be length-unit-divergent, got %v", c.field, f.SourceField.LengthUnitDivergentIDs)
		}
		raw := rawFieldByName(d, c.field)
		fns := validatorsOf[string](raw)
		if len(fns) != 1 {
			t.Fatalf("%s: want exactly 1 string validator, got %d", c.field, len(fns))
		}
		if err := fns[0](c.accept); err != nil {
			t.Fatalf("%s: want %q accepted, got %v", c.field, c.accept, err)
		}
		if err := fns[0](c.reject); err == nil {
			t.Fatalf("%s: want %q rejected", c.field, c.reject)
		}
	}

	// len_bytes_field: len_bytes = 6 -> exactly 6 bytes required. HasLen()
	// (and HasLenBytes()) translate to a MinLen(n).MaxLen(n) PAIR — two
	// separate one-sided closures, not a single combined one, since
	// MinLen/MaxLen each independently append their own validator.
	f := fieldByName(d, "len_bytes_field")
	if f == nil || !contains(f.SourceField.TranslatedIDs, "string.len_bytes") {
		t.Fatalf("len_bytes_field: want string.len_bytes translated, got %+v", f)
	}
	raw := rawFieldByName(d, "len_bytes_field")
	fns := validatorsOf[string](raw)
	if len(fns) != 2 {
		t.Fatalf("len_bytes_field: want exactly 2 validators (MinLen+MaxLen pair), got %d", len(fns))
	}
	if err := allPass(fns, "123456"); err != nil {
		t.Fatalf("len_bytes_field: want 6-byte value accepted, got %v", err)
	}
	if err := allPass(fns, "12345"); err == nil {
		t.Fatal("len_bytes_field: want 5-byte value rejected")
	}
}

// allPass runs every validator in fns against v, returning the first
// error encountered (or nil if every validator accepts v) — used where a
// single protovalidate rule (len/len_bytes) translates to a MinLen+MaxLen
// PAIR of independent ent validators rather than one combined closure.
func allPass[T any](fns []func(T) error, v T) error {
	for _, fn := range fns {
		if err := fn(v); err != nil {
			return err
		}
	}
	return nil
}

// TestTier1StringCodePointBounds proves Task 1's resolved option-c
// decision end to end: min_len/max_len/len are mapped directly onto
// ent's byte-comparing MinLen/MaxLen (translated, and flagged in
// LengthUnitDivergentIDs so the divergence is machine-visible, not
// documentation-only), and the resolution's own worked example — 3 emoji
// are 3 code points but 12 bytes, so protovalidate's max_len: 5 accepts
// while the derived MaxLen(5) rejects — is proven against a REAL
// protovalidate.Validate() call, not merely asserted.
func TestTier1StringCodePointBounds(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.StringCodePointBounds](t)

	for _, tc := range []struct{ field, id string }{
		{"min_len_field", "string.min_len"},
		{"max_len_field", "string.max_len"},
		{"len_field", "string.len"},
	} {
		f := fieldByName(d, tc.field)
		if f == nil || f.SourceField == nil {
			t.Fatalf("%s: want a derived field with a SourceField annotation", tc.field)
		}
		if !contains(f.SourceField.TranslatedIDs, tc.id) {
			t.Fatalf("%s: want %q in TranslatedIDs, got %v", tc.field, tc.id, f.SourceField.TranslatedIDs)
		}
		if !contains(f.SourceField.LengthUnitDivergentIDs, tc.id) {
			t.Fatalf("%s: want %q recorded machine-visibly in LengthUnitDivergentIDs (Task 1 resolution), got %v", tc.field, tc.id, f.SourceField.LengthUnitDivergentIDs)
		}
	}

	// ASCII case: code points == bytes, so protovalidate and the derived
	// ent field must agree (no divergence when there is nothing to
	// diverge over). min_len_field/len_field are set to values that
	// satisfy THEIR OWN constraints (min_len: 3, len: 4) so
	// protovalidate.Validate — which checks the whole message — reports
	// only on max_len_field, the one field under test here.
	raw := rawFieldByName(d, "max_len_field")
	fns := validatorsOf[string](raw)
	if len(fns) != 1 {
		t.Fatalf("max_len_field: want exactly 1 validator, got %d", len(fns))
	}
	asciiOK := "abcde" // 5 code points, 5 bytes
	asciiBad := "abcdef"
	v, err := protovalidate.New()
	if err != nil {
		t.Fatal(err)
	}
	baseline := func(maxLen string) *mixinforprototestv1.StringCodePointBounds {
		return &mixinforprototestv1.StringCodePointBounds{
			MinLenField: "abc",  // satisfies min_len: 3
			LenField:    "abcd", // satisfies len: 4
			MaxLenField: maxLen,
		}
	}
	if verr := v.Validate(baseline(asciiOK)); verr != nil {
		t.Fatalf("protovalidate: want %q accepted, got %v", asciiOK, verr)
	}
	if err := fns[0](asciiOK); err != nil {
		t.Fatalf("ent: want %q accepted, got %v", asciiOK, err)
	}
	if verr := v.Validate(baseline(asciiBad)); verr == nil {
		t.Fatalf("protovalidate: want %q rejected", asciiBad)
	}
	if err := fns[0](asciiBad); err == nil {
		t.Fatalf("ent: want %q rejected", asciiBad)
	}

	// Non-ASCII worked example (Task 1 resolution's own text): 3 emoji
	// are 3 code points but 12 bytes. protovalidate (code points) ACCEPTS
	// at max_len: 5; ent's MaxLen(5) (bytes) REJECTS. This divergence is
	// the documented, deliberate consequence of option-c — not a bug.
	threeEmoji := "\U0001F600\U0001F600\U0001F600" // U+1F600 GRINNING FACE, 4 bytes each in UTF-8
	if verr := v.Validate(baseline(threeEmoji)); verr != nil {
		t.Fatalf("protovalidate: want the 3-emoji value accepted (3 code points <= max_len 5), got %v", verr)
	}
	if err := fns[0](threeEmoji); err == nil {
		t.Fatal("ent: want the 3-emoji value rejected (12 bytes > MaxLen 5) — this is the documented length-unit divergence, not a regression")
	}
}

// TestPatternCompiles proves T-01-01's defensive-compile requirement
// directly against applyStringConstraints: a valid pattern (proven via
// the real StringPattern.valid_pattern corpus field) yields a Match
// validator; an invalid one yields a collected error naming the
// expression, never a MustCompile panic.
//
// The invalid-pattern half is NOT driven through a real corpus .proto
// field: verified this session that buf itself refuses to compile a
// .proto whose buf.validate.field.string.pattern value fails RE2
// compilation (a predefined CEL check on validate.proto's own pattern
// field runs at buf-build time, independently of mixinforproto), so an
// invalid pattern can never reach a real derived Go field descriptor at
// all — see constraints.proto's StringPattern doc comment. This test
// instead exercises applyStringConstraints directly against a hand-built
// fakeStringRules value (defined below, package-local, no import of the
// unspellable *validate.StringRules type needed — see validate.go's file
// doc comment for why). The message/field naming derive.go wraps every
// mapField error with (D-08) is proven generically by
// failure_test.go's TestSchemaLoadFailures already; this test's job is
// only to prove Tier 1's own error path is defensive and names the
// expression.
func TestPatternCompiles(t *testing.T) {
	t.Run("valid pattern derives a Match validator", func(t *testing.T) {
		d := mustDerive[*mixinforprototestv1.StringPattern](t)
		f := fieldByName(d, "valid_pattern")
		if f == nil || !contains(f.SourceField.TranslatedIDs, "string.pattern") {
			t.Fatalf("want string.pattern translated, got %+v", f)
		}
		raw := rawFieldByName(d, "valid_pattern")
		fns := validatorsOf[string](raw)
		if len(fns) != 1 {
			t.Fatalf("want exactly 1 validator, got %d", len(fns))
		}
		if err := fns[0]("abc"); err != nil {
			t.Fatalf("want \"abc\" accepted by ^[a-z]+$, got %v", err)
		}
		if err := fns[0]("ABC"); err == nil {
			t.Fatal("want \"ABC\" rejected by ^[a-z]+$")
		}
	})

	t.Run("invalid pattern is a collected error, never a panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("want an error, got a panic: %v", r)
			}
		}()
		sb := field.String("value")
		_, _, _, err := applyStringConstraints(sb, fakeStringRules{
			hasPattern: true,
			pattern:    "(unterminated[",
		})
		if err == nil {
			t.Fatal("want an error for an invalid pattern")
		}
		if !strings.Contains(err.Error(), "(unterminated[") {
			t.Fatalf("error %q must name the offending expression", err.Error())
		}
	})
}

// --- VAL-03: required/presence translation --------------------------------

func TestRequiredTranslation(t *testing.T) {
	t.Run("string: yields NotEmpty", func(t *testing.T) {
		d := mustDerive[*mixinforprototestv1.RequiredString](t)
		f := fieldByName(d, "value")
		if f == nil || !contains(f.SourceField.TranslatedIDs, "required") {
			t.Fatalf("want required translated, got %+v", f)
		}
		raw := rawFieldByName(d, "value")
		fns := validatorsOf[string](raw)
		if len(fns) != 1 {
			t.Fatalf("want exactly 1 validator (NotEmpty), got %d", len(fns))
		}
		if err := fns[0](""); err == nil {
			t.Fatal("want the empty string rejected")
		}
		if err := fns[0]("x"); err != nil {
			t.Fatalf("want a non-empty string accepted, got %v", err)
		}
	})

	t.Run("optional non-string: yields non-optional construction", func(t *testing.T) {
		d := mustDerive[*mixinforprototestv1.RequiredOptionalNonString](t)
		f := fieldByName(d, "value")
		if f == nil {
			t.Fatal("want a derived field named value")
		}
		if !contains(f.SourceField.TranslatedIDs, "required") {
			t.Fatalf("want required translated, got %+v", f.SourceField)
		}
		if f.Nillable || f.Optional {
			t.Fatalf("want Nillable=false Optional=false (non-optional construction), got Nillable=%v Optional=%v", f.Nillable, f.Optional)
		}
		if f.Default != "" {
			t.Fatalf("want no Default set (a bare required field), got %q", f.Default)
		}
	})

	t.Run("plain non-string: recorded residual, not approximated", func(t *testing.T) {
		d := mustDerive[*mixinforprototestv1.RequiredPlainNonString](t)
		f := fieldByName(d, "value")
		if f == nil {
			t.Fatal("want a derived field named value")
		}
		if !contains(f.SourceField.ResidualIDs, "required") {
			t.Fatalf("want required recorded residual, got %+v", f.SourceField)
		}
		if contains(f.SourceField.TranslatedIDs, "required") {
			t.Fatalf("want required NOT translated (VAL-03's flagged ambiguity), got %+v", f.SourceField)
		}
		// The zero value must remain admissible — no NotEmpty-equivalent
		// exists for a bare int32 field, and Tier 1 must not approximate
		// one. Default(0) is still applied, exactly like an ordinary
		// plain scalar (MIX-05).
		if f.Default == "" {
			t.Fatalf("want plain MIX-05 Default(0) behavior preserved, got no default: %+v", f)
		}
	})

	// --- Gap 3 (VAL-03 / 01-VERIFICATION.md): a presence-tracking
	// (`optional`) string/bytes field carrying `required` must derive
	// PRESENCE semantics (non-optional construction, no NotEmpty), not
	// NON-EMPTINESS semantics. classifyRequired's pre-fix branch order
	// tested `required && hasNotEmpty` before `optional && required`, so
	// this case was unreachable for string/bytes (whose builders always
	// pass hasNotEmpty=true) and instead derived NotEmpty(), rejecting a
	// deliberately-set empty string that protovalidate's own "must be
	// set" verdict accepts.

	t.Run("optional string: yields presence, not NotEmpty", func(t *testing.T) {
		d := mustDerive[*mixinforprototestv1.RequiredOptionalString](t)
		f := fieldByName(d, "value")
		if f == nil {
			t.Fatal("want a derived field named value")
		}
		if f.Nillable || f.Optional {
			t.Fatalf("want Nillable=false Optional=false (non-optional construction), got Nillable=%v Optional=%v", f.Nillable, f.Optional)
		}
		if f.Default != "" {
			t.Fatalf("want no Default set (a bare required field), got %q", f.Default)
		}
		if f.ValidatorCount != 0 {
			t.Fatalf("want zero validators (presence, not NotEmpty), got %d", f.ValidatorCount)
		}
		if !contains(f.SourceField.TranslatedIDs, "required") {
			t.Fatalf("want required translated, got %+v", f.SourceField)
		}
		// The ent layer must admit a deliberately-set empty string,
		// matching protovalidate's own nil verdict on that input.
		raw := rawFieldByName(d, "value")
		for _, fn := range validatorsOf[string](raw) {
			if err := fn(""); err != nil {
				t.Fatalf("want a deliberately-set empty string admissible at the ent layer (matching protovalidate's nil verdict), got %v", err)
			}
		}
	})

	t.Run("optional bytes: yields presence, not NotEmpty", func(t *testing.T) {
		d := mustDerive[*mixinforprototestv1.RequiredOptionalBytes](t)
		f := fieldByName(d, "value")
		if f == nil {
			t.Fatal("want a derived field named value")
		}
		if f.Nillable || f.Optional {
			t.Fatalf("want Nillable=false Optional=false (non-optional construction), got Nillable=%v Optional=%v", f.Nillable, f.Optional)
		}
		if f.Default != "" {
			t.Fatalf("want no Default set (a bare required field), got %q", f.Default)
		}
		if f.ValidatorCount != 0 {
			t.Fatalf("want zero validators (presence, not NotEmpty), got %d", f.ValidatorCount)
		}
		if !contains(f.SourceField.TranslatedIDs, "required") {
			t.Fatalf("want required translated, got %+v", f.SourceField)
		}
		raw := rawFieldByName(d, "value")
		for _, fn := range validatorsOf[[]byte](raw) {
			if err := fn([]byte{}); err != nil {
				t.Fatalf("want a deliberately-set empty byte slice admissible at the ent layer (matching protovalidate's nil verdict), got %v", err)
			}
		}
	})
}

// --- D-11: residual recording ---------------------------------------------

func TestResidualRecorded(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.ResidualCel](t)
	f := fieldByName(d, "value")
	if f == nil {
		t.Fatal("want a derived field named value")
	}
	foundCel := false
	for _, id := range f.SourceField.ResidualIDs {
		if strings.HasPrefix(id, "cel.") {
			foundCel = true
		}
	}
	if !foundCel {
		t.Fatalf("want a cel.* residual ID, got %v", f.SourceField.ResidualIDs)
	}
	if f.SourceField.ResidualFingerprint == "" {
		t.Fatal("want a non-empty ResidualFingerprint")
	}
	if f.ValidatorCount != 0 {
		t.Fatalf("want no builder call for an untranslatable CEL rule, got %d validators", f.ValidatorCount)
	}
}

func TestResidualFingerprintStable(t *testing.T) {
	var fingerprints []string
	for i := 0; i < 5; i++ {
		d := mustDerive[*mixinforprototestv1.ResidualCel](t)
		f := fieldByName(d, "value")
		fingerprints = append(fingerprints, f.SourceField.ResidualFingerprint)
	}
	for i := 1; i < len(fingerprints); i++ {
		if fingerprints[i] != fingerprints[0] {
			t.Fatalf("fingerprint not stable across runs: run 0 = %q, run %d = %q", fingerprints[0], i, fingerprints[i])
		}
	}

	// Changes when the expression text changes: compare against a
	// different residual field's fingerprint (Int32Overflow.gt_max_field
	// carries different residual text entirely).
	d2 := mustDerive[*mixinforprototestv1.Int32Overflow](t)
	other := fieldByName(d2, "gt_max_field")
	if other.SourceField.ResidualFingerprint == fingerprints[0] {
		t.Fatal("want a different fingerprint for different residual expression text")
	}
}

func TestConstraintIDsSorted(t *testing.T) {
	if got := sortUnique([]string{"z", "a", "m", "a"}); !equalStrings(got, []string{"a", "m", "z"}) {
		t.Fatalf("sortUnique: want [a m z], got %v", got)
	}
	if got := sortUnique(nil); len(got) != 0 {
		t.Fatalf("sortUnique(nil): want empty slice, got %v", got)
	}

	// A real derived field's ID lists must also be sorted.
	d := mustDerive[*mixinforprototestv1.Int32Comparators](t)
	f := fieldByName(d, "range_field")
	if !sort.StringsAreSorted(f.SourceField.TranslatedIDs) {
		t.Fatalf("range_field.TranslatedIDs not sorted: %v", f.SourceField.TranslatedIDs)
	}
}

func TestEnumDefinedOnlyNotResidual(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.EnumDefinedOnlyField](t)
	f := fieldByName(d, "status")
	if f == nil {
		t.Fatal("want a derived field named status")
	}
	if len(f.SourceField.ResidualIDs) != 0 {
		t.Fatalf("want defined_only to produce no residual entry (D-14), got %v", f.SourceField.ResidualIDs)
	}
	if len(f.SourceField.TranslatedIDs) != 0 {
		t.Fatalf("want defined_only to produce no translated entry either — it is not tracked at all (D-14), got %v", f.SourceField.TranslatedIDs)
	}
}

// --- VAL-02: numeric translation -------------------------------------------

func TestNumericBoundaries(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.Int32Comparators](t)

	assertVerdicts := func(field string, at4, at5, at6 bool) {
		raw := rawFieldByName(d, field)
		fns := validatorsOf[int32](raw)
		if len(fns) != 1 {
			t.Fatalf("%s: want exactly 1 validator, got %d", field, len(fns))
		}
		check := func(v int32, wantAccept bool) {
			err := fns[0](v)
			if wantAccept && err != nil {
				t.Fatalf("%s: want %d accepted, got %v", field, v, err)
			}
			if !wantAccept && err == nil {
				t.Fatalf("%s: want %d rejected", field, v)
			}
		}
		check(4, at4)
		check(5, at5)
		check(6, at6)
	}

	// gt: 5 -> Min(6): 4 rejected, 5 rejected, 6 accepted.
	assertVerdicts("gt_field", false, false, true)
	// gte: 5 -> Min(5): 4 rejected, 5 accepted, 6 accepted.
	assertVerdicts("gte_field", false, true, true)
	// lt: 5 -> Max(4): 4 accepted, 5 rejected, 6 rejected.
	assertVerdicts("lt_field", true, false, false)
	// lte: 5 -> Max(5): 4 accepted, 5 accepted, 6 rejected.
	assertVerdicts("lte_field", true, true, false)

	for _, field := range []string{"gt_field", "gte_field", "lt_field", "lte_field"} {
		f := fieldByName(d, field)
		if len(f.SourceField.TranslatedIDs) != 1 || len(f.SourceField.ResidualIDs) != 0 {
			t.Fatalf("%s: want exactly 1 translated ID and 0 residual, got %+v", field, f.SourceField)
		}
	}
}

func TestNumericRangeCombination(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.Int32Comparators](t)
	f := fieldByName(d, "range_field")
	if !contains(f.SourceField.TranslatedIDs, "int32.gt") || !contains(f.SourceField.TranslatedIDs, "int32.lt") {
		t.Fatalf("want both int32.gt and int32.lt translated, got %v", f.SourceField.TranslatedIDs)
	}
	raw := rawFieldByName(d, "range_field")
	fns := validatorsOf[int32](raw)
	if len(fns) != 1 {
		t.Fatalf("want exactly 1 validator (a single Range call), got %d", len(fns))
	}
	if err := fns[0](2); err == nil {
		t.Fatal("want 2 rejected")
	}
	if err := fns[0](3); err != nil {
		t.Fatalf("want 3 accepted, got %v", err)
	}
	if err := fns[0](8); err != nil {
		t.Fatalf("want 8 accepted, got %v", err)
	}
	if err := fns[0](9); err == nil {
		t.Fatal("want 9 rejected")
	}
}

func TestNumericAdjacentBounds(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.Int32Adjacent](t)

	t.Run("coincident: exactly one admissible value", func(t *testing.T) {
		raw := rawFieldByName(d, "coincident_field")
		fns := validatorsOf[int32](raw)
		if len(fns) != 1 {
			t.Fatalf("want exactly 1 validator, got %d", len(fns))
		}
		if err := fns[0](5); err != nil {
			t.Fatalf("want 5 accepted, got %v", err)
		}
		if err := fns[0](4); err == nil {
			t.Fatal("want 4 rejected")
		}
		if err := fns[0](6); err == nil {
			t.Fatal("want 6 rejected")
		}
	})

	t.Run("crossed: no admissible value", func(t *testing.T) {
		raw := rawFieldByName(d, "crossed_field")
		fns := validatorsOf[int32](raw)
		if len(fns) != 1 {
			t.Fatalf("want exactly 1 validator, got %d", len(fns))
		}
		for _, v := range []int32{4, 5, 6, 7} {
			if err := fns[0](v); err == nil {
				t.Fatalf("want %d rejected (crossed range admits nothing)", v)
			}
		}
	})
}

func TestNumericOverflowIsResidual(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.Int32Overflow](t)

	for _, tc := range []struct{ field, id string }{
		{"gt_max_field", "int32.gt"},
		{"lt_min_field", "int32.lt"},
	} {
		f := fieldByName(d, tc.field)
		if f == nil {
			t.Fatalf("want a derived field named %s", tc.field)
		}
		if !contains(f.SourceField.ResidualIDs, tc.id) {
			t.Fatalf("%s: want %q residual, got %v", tc.field, tc.id, f.SourceField.ResidualIDs)
		}
		if contains(f.SourceField.TranslatedIDs, tc.id) {
			t.Fatalf("%s: want %q NOT translated, got %v", tc.field, tc.id, f.SourceField.TranslatedIDs)
		}
		if f.ValidatorCount != 0 {
			t.Fatalf("%s: want no Min/Max call emitted, got %d validators", tc.field, f.ValidatorCount)
		}
	}
}

func TestFloatGtLtIsResidual(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.FloatComparators](t)
	for _, field := range []string{"gt_field", "lt_field"} {
		f := fieldByName(d, field)
		if len(f.SourceField.TranslatedIDs) != 0 {
			t.Fatalf("%s: want no translated IDs, got %v", field, f.SourceField.TranslatedIDs)
		}
		if len(f.SourceField.ResidualIDs) != 1 {
			t.Fatalf("%s: want exactly 1 residual ID, got %v", field, f.SourceField.ResidualIDs)
		}
		if f.ValidatorCount != 0 {
			t.Fatalf("%s: want no Min/Max call, got %d", field, f.ValidatorCount)
		}
	}

	dd := mustDerive[*mixinforprototestv1.DoubleComparators](t)
	f := fieldByName(dd, "gt_field")
	if !contains(f.SourceField.ResidualIDs, "double.gt") {
		t.Fatalf("want double.gt residual, got %v", f.SourceField.ResidualIDs)
	}
	if f.ValidatorCount != 0 {
		t.Fatalf("want no Min/Max call for double.gt, got %d", f.ValidatorCount)
	}
}

func TestFloatGteLteTranslates(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.FloatComparators](t)
	f := fieldByName(d, "gte_lte_field")
	if !contains(f.SourceField.TranslatedIDs, "float.gte") || !contains(f.SourceField.TranslatedIDs, "float.lte") {
		t.Fatalf("want float.gte and float.lte translated, got %v", f.SourceField.TranslatedIDs)
	}
	raw := rawFieldByName(d, "gte_lte_field")
	fns := validatorsOf[float32](raw)
	if len(fns) != 1 {
		t.Fatalf("want exactly 1 validator, got %d", len(fns))
	}
	if err := fns[0](1.5); err != nil {
		t.Fatalf("want 1.5 accepted, got %v", err)
	}
	if err := fns[0](9.5); err != nil {
		t.Fatalf("want 9.5 accepted, got %v", err)
	}
	if err := fns[0](1.4); err == nil {
		t.Fatal("want 1.4 rejected")
	}
	if err := fns[0](9.6); err == nil {
		t.Fatal("want 9.6 rejected")
	}

	dd := mustDerive[*mixinforprototestv1.DoubleComparators](t)
	fd := fieldByName(dd, "gte_lte_field")
	if !contains(fd.SourceField.TranslatedIDs, "double.gte") || !contains(fd.SourceField.TranslatedIDs, "double.lte") {
		t.Fatalf("want double.gte and double.lte translated, got %v", fd.SourceField.TranslatedIDs)
	}
}

func TestPositiveTranslation(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.Int32Comparators](t)
	raw := rawFieldByName(d, "positive_field")
	fns := validatorsOf[int32](raw)
	if len(fns) != 1 {
		t.Fatalf("want exactly 1 validator, got %d", len(fns))
	}
	// gt: 0 must verdict-match Min(1): 0 rejected, 1 accepted.
	if err := fns[0](0); err == nil {
		t.Fatal("want 0 rejected")
	}
	if err := fns[0](1); err != nil {
		t.Fatalf("want 1 accepted, got %v", err)
	}
}

func TestNumericNoRules(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.Int32Comparators](t)
	f := fieldByName(d, "no_rules_field")
	if f.ValidatorCount != 0 {
		t.Fatalf("want 0 validators for a rules-free numeric field, got %d", f.ValidatorCount)
	}
	if len(f.SourceField.TranslatedIDs) != 0 || len(f.SourceField.ResidualIDs) != 0 {
		t.Fatalf("want empty TranslatedIDs/ResidualIDs, got %+v", f.SourceField)
	}
}

// TestNumericConstraintsNeverInNeither is this plan's own closing
// acceptance criterion for VAL-02: "the SourceField annotation for every
// numeric corpus field lists each constraint in exactly one of
// TranslatedIDs or ResidualIDs, never in neither." For every gt/gte/lt/lte
// this plan's numeric corpus messages resolve, the corresponding
// constraint ID (kindName + "." + comparator) is asserted present in
// exactly the union of TranslatedIDs and ResidualIDs, and never in both
// at once (a constraint is either translated or residual, not both).
func TestNumericConstraintsNeverInNeither(t *testing.T) {
	type fieldCase struct {
		field    string
		kindName string
		hasGt    bool
		hasGte   bool
		hasLt    bool
		hasLte   bool
	}
	check := func(t *testing.T, d *derivation, c fieldCase) {
		t.Helper()
		f := fieldByName(d, c.field)
		if f == nil {
			t.Fatalf("%s: want a derived field", c.field)
		}
		want := map[string]bool{}
		if c.hasGt {
			want[c.kindName+".gt"] = true
		}
		if c.hasGte {
			want[c.kindName+".gte"] = true
		}
		if c.hasLt {
			want[c.kindName+".lt"] = true
		}
		if c.hasLte {
			want[c.kindName+".lte"] = true
		}
		for id := range want {
			inTranslated := contains(f.SourceField.TranslatedIDs, id)
			inResidual := contains(f.SourceField.ResidualIDs, id)
			if !inTranslated && !inResidual {
				t.Fatalf("%s: constraint %q is in NEITHER TranslatedIDs %v nor ResidualIDs %v", c.field, id, f.SourceField.TranslatedIDs, f.SourceField.ResidualIDs)
			}
			if inTranslated && inResidual {
				t.Fatalf("%s: constraint %q is in BOTH TranslatedIDs and ResidualIDs", c.field, id)
			}
		}
	}

	d := mustDerive[*mixinforprototestv1.Int32Comparators](t)
	for _, c := range []fieldCase{
		{"gt_field", "int32", true, false, false, false},
		{"gte_field", "int32", false, true, false, false},
		{"lt_field", "int32", false, false, true, false},
		{"lte_field", "int32", false, false, false, true},
		{"positive_field", "int32", true, false, false, false},
		{"range_field", "int32", true, false, true, false},
	} {
		check(t, d, c)
	}

	dOv := mustDerive[*mixinforprototestv1.Int32Overflow](t)
	check(t, dOv, fieldCase{"gt_max_field", "int32", true, false, false, false})
	check(t, dOv, fieldCase{"lt_min_field", "int32", false, false, true, false})

	dF := mustDerive[*mixinforprototestv1.FloatComparators](t)
	check(t, dF, fieldCase{"gt_field", "float", true, false, false, false})
	check(t, dF, fieldCase{"lt_field", "float", false, false, true, false})
	check(t, dF, fieldCase{"gte_lte_field", "float", false, true, false, true})

	dD := mustDerive[*mixinforprototestv1.DoubleComparators](t)
	check(t, dD, fieldCase{"gt_field", "double", true, false, false, false})
	check(t, dD, fieldCase{"gte_lte_field", "double", false, true, false, true})
}

// --- D-13: format validators -----------------------------------------------

func TestStringFormatValidators(t *testing.T) {
	cases := []struct {
		msgType      string
		derive       func() (*derivation, error)
		wantID       string
		accept, deny string
	}{
		{"Email", func() (*derivation, error) { return derive[*mixinforprototestv1.StringFormatEmail]() }, "string.email", "foo@example.com", "not-an-email"},
		{"Hostname", func() (*derivation, error) { return derive[*mixinforprototestv1.StringFormatHostname]() }, "string.hostname", "example.com", "not a hostname!!"},
		{"Uri", func() (*derivation, error) { return derive[*mixinforprototestv1.StringFormatUri]() }, "string.uri", "https://example.com/foo", "not a uri"},
		{"Ip", func() (*derivation, error) { return derive[*mixinforprototestv1.StringFormatIp]() }, "string.ip", "127.0.0.1", "not-an-ip"},
		{"Uuid", func() (*derivation, error) { return derive[*mixinforprototestv1.StringFormatUuid]() }, "string.uuid", "123e4567-e89b-12d3-a456-426614174000", "not-a-uuid"},
	}
	for _, c := range cases {
		t.Run(c.msgType, func(t *testing.T) {
			d, err := c.derive()
			if err != nil {
				t.Fatalf("derive: %v", err)
			}
			f := fieldByName(d, "value")
			if f == nil || !contains(f.SourceField.TranslatedIDs, c.wantID) {
				t.Fatalf("want %q translated, got %+v", c.wantID, f)
			}
			raw := rawFieldByName(d, "value")
			fns := validatorsOf[string](raw)
			if len(fns) != 1 {
				t.Fatalf("want exactly 1 validator, got %d", len(fns))
			}
			if err := fns[0](c.accept); err != nil {
				t.Fatalf("want %q accepted, got %v", c.accept, err)
			}
			if err := fns[0](c.deny); err == nil {
				t.Fatalf("want %q rejected", c.deny)
			}
		})
	}
}

// TestDelegatedFormatIgnoresUnrelatedViolations proves 01-VERIFICATION.md
// gap 2 / 01-REVIEW.md CR-02 is closed: delegatingFormatValidator must
// filter protovalidate's whole-message verdict down to the violation
// attributed to the field under validation, so an unrelated rule
// elsewhere on the same message (StringFormatWithSibling's `owner`,
// carrying `required`) never poisons `endpoint`'s format verdict. Both
// halves are mandatory — the second is the anti-bypass guard: a filter
// that is too aggressive (e.g. matching no violation at all) would turn
// the validator into a no-op, silently admitting invalid data, which is
// strictly worse than the bug being fixed (T-01G-10).
func TestDelegatedFormatIgnoresUnrelatedViolations(t *testing.T) {
	d := mustDerive[*mixinforprototestv1.StringFormatWithSibling](t)

	endpoint := fieldByName(d, "endpoint")
	if endpoint == nil || !contains(endpoint.SourceField.TranslatedIDs, "string.uri") {
		t.Fatalf("want string.uri translated on endpoint, got %+v", endpoint)
	}
	rawEndpoint := rawFieldByName(d, "endpoint")
	endpointFns := validatorsOf[string](rawEndpoint)
	if len(endpointFns) != 1 {
		t.Fatalf("want exactly 1 validator on endpoint, got %d", len(endpointFns))
	}

	// The gap: a valid URI must be accepted even though the sibling
	// field `owner` (unset, zero value) carries an unrelated `required`
	// rule and would independently produce a whole-message violation.
	// Before the fix this FAILS: the unfiltered whole-message verdict
	// blames endpoint for owner's violation.
	if err := endpointFns[0]("https://example.com/foo"); err != nil {
		t.Fatalf("want a valid URI accepted despite the unrelated required violation on owner, got %v", err)
	}

	// The anti-bypass guard: an actually-invalid URI on the SAME
	// multi-field message must still be rejected. A filter that matches
	// no violation (e.g. because FieldDescriptor comparison is wrong)
	// would make this pass silently for the wrong reason — the validator
	// would have become a no-op.
	if err := endpointFns[0]("not a uri"); err == nil {
		t.Fatal("want an invalid URI still rejected on a multi-field message — a permissive filter is a validation bypass, not a fix")
	}

	// owner (plain string + required, implicit presence — untouched by
	// this plan's Task 2 reorder) still derives NotEmpty().
	owner := fieldByName(d, "owner")
	if owner == nil || !contains(owner.SourceField.TranslatedIDs, "required") {
		t.Fatalf("want required translated on owner, got %+v", owner)
	}
	rawOwner := rawFieldByName(d, "owner")
	ownerFns := validatorsOf[string](rawOwner)
	if len(ownerFns) != 1 {
		t.Fatalf("want exactly 1 validator on owner (NotEmpty), got %d", len(ownerFns))
	}
	if err := ownerFns[0](""); err == nil {
		t.Fatal("want owner's NotEmpty to still reject the empty string")
	}
	if err := ownerFns[0]("someone"); err != nil {
		t.Fatalf("want owner's NotEmpty to still accept a non-empty string, got %v", err)
	}
}

// --- D-23/R4: golden fixtures for the constraints.proto corpus -----------

// TestGoldenConstraints golden-asserts one fixture per constraints.proto
// message (D-23, this plan's own must_haves artifact list), reusing
// fieldmap_test.go's assertGolden/fieldByName helpers and projection —
// never marshaling the raw *field.Descriptor (R4).
func TestGoldenConstraints(t *testing.T) {
	t.Run("NoRules", func(t *testing.T) {
		assertGolden(t, "constraints_no_rules", derive[*mixinforprototestv1.NoRules])
	})
	t.Run("StringByteBounds", func(t *testing.T) {
		assertGolden(t, "constraints_string_byte_bounds", derive[*mixinforprototestv1.StringByteBounds])
	})
	t.Run("StringCodePointBounds", func(t *testing.T) {
		assertGolden(t, "constraints_string_codepoint_bounds", derive[*mixinforprototestv1.StringCodePointBounds])
	})
	t.Run("StringPattern", func(t *testing.T) {
		assertGolden(t, "constraints_string_pattern", derive[*mixinforprototestv1.StringPattern])
	})
	t.Run("StringFormatEmail", func(t *testing.T) {
		assertGolden(t, "constraints_string_format_email", derive[*mixinforprototestv1.StringFormatEmail])
	})
	t.Run("StringFormatHostname", func(t *testing.T) {
		assertGolden(t, "constraints_string_format_hostname", derive[*mixinforprototestv1.StringFormatHostname])
	})
	t.Run("StringFormatUri", func(t *testing.T) {
		assertGolden(t, "constraints_string_format_uri", derive[*mixinforprototestv1.StringFormatUri])
	})
	t.Run("StringFormatIp", func(t *testing.T) {
		assertGolden(t, "constraints_string_format_ip", derive[*mixinforprototestv1.StringFormatIp])
	})
	t.Run("StringFormatUuid", func(t *testing.T) {
		assertGolden(t, "constraints_string_format_uuid", derive[*mixinforprototestv1.StringFormatUuid])
	})
	t.Run("RequiredString", func(t *testing.T) {
		assertGolden(t, "constraints_required_string", derive[*mixinforprototestv1.RequiredString])
	})
	t.Run("RequiredOptionalNonString", func(t *testing.T) {
		assertGolden(t, "constraints_required_optional_non_string", derive[*mixinforprototestv1.RequiredOptionalNonString])
	})
	t.Run("RequiredPlainNonString", func(t *testing.T) {
		assertGolden(t, "constraints_required_plain_non_string", derive[*mixinforprototestv1.RequiredPlainNonString])
	})
	t.Run("ResidualCel", func(t *testing.T) {
		assertGolden(t, "constraints_residual_cel", derive[*mixinforprototestv1.ResidualCel])
	})
	t.Run("EnumDefinedOnlyField", func(t *testing.T) {
		assertGolden(t, "constraints_enum_defined_only", derive[*mixinforprototestv1.EnumDefinedOnlyField])
	})
	t.Run("Int32Comparators", func(t *testing.T) {
		assertGolden(t, "constraints_int32_comparators", derive[*mixinforprototestv1.Int32Comparators])
	})
	t.Run("Int32Adjacent", func(t *testing.T) {
		assertGolden(t, "constraints_int32_adjacent", derive[*mixinforprototestv1.Int32Adjacent])
	})
	t.Run("Int32Overflow", func(t *testing.T) {
		assertGolden(t, "constraints_int32_overflow", derive[*mixinforprototestv1.Int32Overflow])
	})
	t.Run("FloatComparators", func(t *testing.T) {
		assertGolden(t, "constraints_float_comparators", derive[*mixinforprototestv1.FloatComparators])
	})
	t.Run("DoubleComparators", func(t *testing.T) {
		assertGolden(t, "constraints_double_comparators", derive[*mixinforprototestv1.DoubleComparators])
	})
}

// --- test-local helpers ----------------------------------------------------

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// fakeStringRules is a package-local, hand-written implementation of
// stringRulesIface (validate.go) — used to unit-test
// applyStringConstraints's defensive pattern-compile path without a real
// *validate.StringRules (which cannot be constructed here without
// violating MIX-14's three-direct-dependency invariant — see validate.go's
// file doc comment). Every method returns its configured value; unset
// fields report Has*()=false.
type fakeStringRules struct {
	hasPattern bool
	pattern    string
}

func (f fakeStringRules) HasMinLen() bool     { return false }
func (f fakeStringRules) GetMinLen() uint64   { return 0 }
func (f fakeStringRules) HasMaxLen() bool     { return false }
func (f fakeStringRules) GetMaxLen() uint64   { return 0 }
func (f fakeStringRules) HasLen() bool        { return false }
func (f fakeStringRules) GetLen() uint64      { return 0 }
func (f fakeStringRules) HasMinBytes() bool   { return false }
func (f fakeStringRules) GetMinBytes() uint64 { return 0 }
func (f fakeStringRules) HasMaxBytes() bool   { return false }
func (f fakeStringRules) GetMaxBytes() uint64 { return 0 }
func (f fakeStringRules) HasLenBytes() bool   { return false }
func (f fakeStringRules) GetLenBytes() uint64 { return 0 }
func (f fakeStringRules) HasPattern() bool    { return f.hasPattern }
func (f fakeStringRules) GetPattern() string  { return f.pattern }
func (f fakeStringRules) HasWellKnown() bool  { return false }
func (f fakeStringRules) GetEmail() bool      { return false }
func (f fakeStringRules) GetHostname() bool   { return false }
func (f fakeStringRules) GetUri() bool        { return false }
func (f fakeStringRules) GetIp() bool         { return false }
func (f fakeStringRules) GetUuid() bool       { return false }
