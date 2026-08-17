package difftest

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"strings"

	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// This file is PIPE-06/Task 3's deterministically seeded value generator
// (D-14). sweep_test.go drives every generated value through both
// protovalidate.Validate (the boundary verdict) and a real ent mutation
// (the storage verdict) via hooks.go's compiled evaluator, so the two
// verdicts are compared on the SAME underlying native Go value —
// generation happens exactly once per (message, field, run), never
// separately per side.

// sweepSeedFor derives sweep_test.go's per-corpus-message PRNG seed from
// md's FullName alone: a pure function of the name, never the clock.
// Two calls for the same name return the identical seed, which is what
// makes go test -count=5 (Phase 1 D-24) a real order-independence check
// rather than a lottery, and what lets a red CI run reproduce locally
// verbatim from the printed seed alone (D-14's own text).
func sweepSeedFor(name protoreflect.FullName) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return int64(h.Sum64()) //nolint:gosec // deterministic hash bits, not a security value
}

// numeric bounds the Go types this file's boundary-value helper works
// over — every proto numeric kind hooks.go's hybrid evaluator can
// currently reach (classScalar/classOptionalScalar, hooks.go's
// hookFieldClass doc comment).
type numeric interface {
	~int32 | ~int64 | ~uint32 | ~uint64 | ~float32 | ~float64
}

// rangeRules is satisfied by every protovalidate numeric Rules message
// this file reads (Int32Rules, SInt32Rules, SFixed32Rules, Int64Rules,
// ..., FloatRules, DoubleRules) — mirroring, independently, the same
// structural-interface technique validate.go (mixinforproto package)
// uses for its own Tier 1 translation; this file cannot import that
// package's unexported interface, so it declares its own with the
// identical method shape.
type rangeRules[V numeric] interface {
	HasGt() bool
	GetGt() V
	HasGte() bool
	GetGte() V
	HasLt() bool
	GetLt() V
	HasLte() bool
	GetLte() V
}

// intBoundaryValues returns bound, bound-1, bound+1 for every gt/gte/
// lt/lte rule present on r — "one representable unit either side of the
// rule's threshold" for an integer kind (D-14/must_haves).
func intBoundaryValues[V numeric](r rangeRules[V]) []V {
	var out []V
	if r.HasGt() {
		n := r.GetGt()
		out = append(out, n-1, n, n+1)
	}
	if r.HasGte() {
		n := r.GetGte()
		out = append(out, n-1, n, n+1)
	}
	if r.HasLt() {
		n := r.GetLt()
		out = append(out, n-1, n, n+1)
	}
	if r.HasLte() {
		n := r.GetLte()
		out = append(out, n-1, n, n+1)
	}
	return out
}

// float32BoundaryValues mirrors intBoundaryValues for float32, using
// math.Nextafter32 for "one representable unit either side" — +/-1 is
// not a representable-unit step for a floating-point kind.
func float32BoundaryValues(r rangeRules[float32]) []float32 {
	var out []float32
	add := func(n float32) {
		out = append(out,
			math.Nextafter32(n, float32(math.Inf(-1))),
			n,
			math.Nextafter32(n, float32(math.Inf(1))),
		)
	}
	if r.HasGt() {
		add(r.GetGt())
	}
	if r.HasGte() {
		add(r.GetGte())
	}
	if r.HasLt() {
		add(r.GetLt())
	}
	if r.HasLte() {
		add(r.GetLte())
	}
	return out
}

// float64BoundaryValues mirrors float32BoundaryValues for float64.
func float64BoundaryValues(r rangeRules[float64]) []float64 {
	var out []float64
	add := func(n float64) {
		out = append(out,
			math.Nextafter(n, math.Inf(-1)),
			n,
			math.Nextafter(n, math.Inf(1)),
		)
	}
	if r.HasGt() {
		add(r.GetGt())
	}
	if r.HasGte() {
		add(r.GetGte())
	}
	if r.HasLt() {
		add(r.GetLt())
	}
	if r.HasLte() {
		add(r.GetLte())
	}
	return out
}

// fieldValueCandidates returns several candidate native Go values for a
// scalar/optionalScalar field fd, appropriate to fd.Kind() (D-14's "per-
// field values appropriate to each derivation kind") and driven by fd's
// OWN resolved protovalidate rules — never a hand-typed per-message
// table, so a corpus field's rule changes are picked up automatically.
// Always includes the type's zero value and a small pseudo-random value
// (from rng, itself seeded from sweepSeedFor — deterministic, D-14).
// When fd carries a numeric or string-length bound, includes boundary-
// adjacent values (at the bound, one representable unit either side).
// When fd carries a well-known string format, includes one format-valid
// and one format-invalid candidate. When divergent is true (the field's
// SourceField.LengthUnitDivergentIDs is non-empty), includes at least
// one multi-byte non-ASCII string whose byte length differs from its
// rune count — the exact divergence README.md flags, exercised rather
// than avoided.
func fieldValueCandidates(rng *rand.Rand, fd protoreflect.FieldDescriptor, divergent bool) []any {
	switch fd.Kind() {
	case protoreflect.StringKind:
		return stringCandidates(rng, fd, divergent)
	case protoreflect.BoolKind:
		return []any{false, true}
	case protoreflect.BytesKind:
		return []any{[]byte(nil), []byte{}, randomBytes(rng, 6)}
	case protoreflect.DoubleKind:
		return float64Candidates(rng, fd)
	case protoreflect.FloatKind:
		return float32Candidates(rng, fd)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return int32Candidates(rng, fd)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return int64Candidates(rng, fd)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return []any{uint32(0), uint32(rng.Int31())} //nolint:gosec
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return []any{uint64(0), uint64(rng.Int63())} //nolint:gosec
	default:
		return nil
	}
}

func stringCandidates(rng *rand.Rand, fd protoreflect.FieldDescriptor, divergent bool) []any {
	out := []any{""}
	if divergent {
		// "café" (Latin-1 accented é, 2 bytes) + 3 emoji (4 bytes each,
		// 1 rune each): rune count (8) differs from byte length (17) —
		// exactly the code-point-vs-byte divergence LengthUnitDivergentIDs
		// flags (protovalidate's string.min_len/max_len/len count code
		// points; ent's MinLen/MaxLen count bytes).
		out = append(out, "café 😀😀😀")
	}
	out = append(out, randomAlnum(rng, 5+rng.Intn(6)))

	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil || rules == nil || rules.GetString() == nil {
		return out
	}
	sr := rules.GetString()

	addLen := func(n int) {
		if n < 0 {
			return
		}
		out = append(out, strings.Repeat("a", n))
	}
	if sr.HasMinLen() {
		n := int(sr.GetMinLen())
		addLen(n - 1)
		addLen(n)
		addLen(n + 1)
	}
	if sr.HasMaxLen() {
		n := int(sr.GetMaxLen())
		addLen(n - 1)
		addLen(n)
		addLen(n + 1)
	}
	if sr.HasLen() {
		n := int(sr.GetLen())
		addLen(n - 1)
		addLen(n)
		addLen(n + 1)
	}
	if sr.HasPattern() {
		out = append(out, "abc", "123 not matching!!")
	}
	if sr.HasWellKnown() {
		switch {
		case sr.GetEmail():
			out = append(out, "user@example.com", "not-an-email")
		case sr.GetHostname():
			out = append(out, "example.com", "not a hostname!!")
		case sr.GetUri():
			out = append(out, "https://example.com", "not a uri")
		case sr.GetIp():
			out = append(out, "127.0.0.1", "999.999.999.999")
		case sr.GetUuid():
			out = append(out, "123e4567-e89b-12d3-a456-426614174000", "not-a-uuid")
		}
	}
	return out
}

func int32Candidates(rng *rand.Rand, fd protoreflect.FieldDescriptor) []any {
	out := []any{int32(0), int32(rng.Int31()%1000 - 500)} //nolint:gosec
	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil || rules == nil {
		return out
	}
	var bv []int32
	switch {
	case rules.HasInt32():
		bv = intBoundaryValues[int32](rules.GetInt32())
	case rules.HasSint32():
		bv = intBoundaryValues[int32](rules.GetSint32())
	case rules.HasSfixed32():
		bv = intBoundaryValues[int32](rules.GetSfixed32())
	}
	for _, v := range bv {
		out = append(out, v)
	}
	return out
}

func int64Candidates(rng *rand.Rand, fd protoreflect.FieldDescriptor) []any {
	out := []any{int64(0), int64(rng.Int63()%1000 - 500)} //nolint:gosec
	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil || rules == nil {
		return out
	}
	var bv []int64
	switch {
	case rules.HasInt64():
		bv = intBoundaryValues[int64](rules.GetInt64())
	case rules.HasSint64():
		bv = intBoundaryValues[int64](rules.GetSint64())
	case rules.HasSfixed64():
		bv = intBoundaryValues[int64](rules.GetSfixed64())
	}
	for _, v := range bv {
		out = append(out, v)
	}
	return out
}

func float32Candidates(rng *rand.Rand, fd protoreflect.FieldDescriptor) []any {
	out := []any{float32(0), float32(rng.Float64()*1000 - 500)}
	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil || rules == nil || !rules.HasFloat() {
		return out
	}
	for _, v := range float32BoundaryValues(rules.GetFloat()) {
		out = append(out, v)
	}
	return out
}

func float64Candidates(rng *rand.Rand, fd protoreflect.FieldDescriptor) []any {
	out := []any{float64(0), rng.Float64()*1000 - 500}
	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil || rules == nil || !rules.HasDouble() {
		return out
	}
	for _, v := range float64BoundaryValues(rules.GetDouble()) {
		out = append(out, v)
	}
	return out
}

const alnumAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomAlnum(rng *rand.Rand, n int) string {
	var sb strings.Builder
	sb.Grow(n)
	for i := 0; i < n; i++ {
		sb.WriteByte(alnumAlphabet[rng.Intn(len(alnumAlphabet))])
	}
	return sb.String()
}

func randomBytes(rng *rand.Rand, n int) []byte {
	b := make([]byte, n)
	_, _ = rng.Read(b)
	return b
}

// formatValue renders v for a disagreement failure message (D-14: the
// seed, message name, field, generated value and both verdicts must all
// print on failure so a red CI reproduces locally verbatim).
func formatValue(v any) string {
	switch x := v.(type) {
	case []byte:
		return fmt.Sprintf("%q (bytes)", x)
	case string:
		return fmt.Sprintf("%q", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
