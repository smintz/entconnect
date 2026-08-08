package mixinforproto

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"buf.build/go/protovalidate"
)

// This file implements Tier 1 (01-05-PLAN.md): the protovalidate
// constraints that have an exact ent equivalent become real ent builder
// calls; everything else is recorded as residual provenance on
// SourceField (D-11) — never enforced, never rejected, never silently
// dropped (T-01-23).
//
// # Why this file never imports buf.build/gen/.../buf/validate
//
// protovalidate.ResolveFieldRules(fd) returns a concrete
// *validate.FieldRules (from
// buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate).
// CORRECTION (01-REVIEW.md WR-06, 01-08-PLAN.md): that package is
// ALREADY a direct, non-test dependency of mixinforproto — go.mod's
// first require block lists it with no `// indirect` marker, alongside
// entgo.io/ent, buf.build/go/protovalidate, github.com/sebdah/goldie/v2
// (test-only) and google.golang.org/protobuf, because the generated
// corpus (constraints.pb.go et al.) imports it directly to reference
// buf.validate's field options. So naming *validate.FieldRules here
// would NOT add a new direct dependency; MIX-14's actual invariant is
// "ent + protobuf + protovalidate toolchain only" (satisfied either
// way), not "exactly three direct dependencies" — that narrower count
// was already false before this file's own indirection was written, not
// something this file's structure protects. This file nonetheless still
// avoids naming that type (see below) — that choice is a real, if
// unpaid-for, dependency-surface preference, not a MIX-14 requirement.
// De-indirecting this layer is a refactor for a future plan, not a gap
// fix; not attempted here.
//
// Every function below therefore either (a) calls methods on a locally
// `:=`-inferred value without ever spelling its type name, or (b) accepts
// the extracted sub-rule as a plain interface parameter typed with a
// small, PRIMITIVE-ONLY interface defined in this file (stringRulesIface,
// rangeRulesIface[V], ...). Go's interface/generic-constraint
// satisfaction is structural: a concrete type from an unimported package
// satisfies an interface whose methods use only primitive/already-
// imported types, with no import of the defining package required. This
// is the same "accept an unexported/unnamed type via inference" trick
// fieldmap.go already uses for field.String(name)'s *stringBuilder
// return value (Plan 01) — applied here one level further, to a type
// this package cannot even name.

// --- Rule-extraction interfaces (primitive-only; see file doc comment) ----

// stringRulesIface is satisfied by protovalidate's *validate.StringRules.
type stringRulesIface interface {
	HasMinLen() bool
	GetMinLen() uint64
	HasMaxLen() bool
	GetMaxLen() uint64
	HasLen() bool
	GetLen() uint64
	HasMinBytes() bool
	GetMinBytes() uint64
	HasMaxBytes() bool
	GetMaxBytes() uint64
	HasLenBytes() bool
	GetLenBytes() uint64
	HasPattern() bool
	GetPattern() string
	HasWellKnown() bool
	GetEmail() bool
	GetHostname() bool
	GetUri() bool
	GetIp() bool
	GetUuid() bool
}

// rangeRulesIface[V] is satisfied by every protovalidate numeric Rules
// message this plan reads (Int32Rules, SInt32Rules, SFixed32Rules,
// Int64Rules, ..., FloatRules, DoubleRules) — they all share this exact
// method shape, differing only in V.
type rangeRulesIface[V any] interface {
	HasGt() bool
	GetGt() V
	HasGte() bool
	GetGte() V
	HasLt() bool
	GetLt() V
	HasLte() bool
	GetLte() V
}

// anyRangeRulesPresence is the presence-only projection of
// rangeRulesIface, satisfied uniformly by every numeric Rules message —
// including the unsigned ones (UInt32Rules/UInt64Rules/Fixed32Rules/
// Fixed64Rules) this plan does not translate. Used to detect and record
// an unsigned interval constraint as residual rather than silently
// dropping it (T-01-23) without needing per-type value extraction.
type anyRangeRulesPresence interface {
	HasGt() bool
	HasGte() bool
	HasLt() bool
	HasLte() bool
}

// --- Builder-side generic interfaces (self-referential; T is inferred) ----

// stringBuilderIface is satisfied by field.String(name)'s returned
// builder (an unexported *stringBuilder — T is always inferred, never
// spelled, exactly as fieldmap.go already relies on elsewhere).
type stringBuilderIface[T any] interface {
	MinLen(int) T
	MaxLen(int) T
	Match(*regexp.Regexp) T
	NotEmpty() T
	Validate(func(string) error) T
}

// numBuilderIface is satisfied by every numeric field builder this plan
// touches (field.Int32/Int64/Float32/Float's returned builders).
type numBuilderIface[T any, V any] interface {
	Min(V) T
	Max(V) T
	Range(V, V) T
}

// signedNum bounds the integer kinds D-12's overflow-guarded adjustment
// applies to in this plan (int32/int64 and their wire-format siblings —
// see fieldmap.go's mapScalar switch, which maps Sint32Kind/Sfixed32Kind
// onto the same field.Int32 builder as Int32Kind, etc.).
type signedNum interface{ ~int32 | ~int64 }

// floatNum bounds the floating-point kinds D-12 always records gt/lt as
// residual for.
type floatNum interface{ ~float32 | ~float64 }

// --- Residual bookkeeping ---------------------------------------------

// residualEntry pairs one residual constraint ID with the text
// fingerprinted for it: the literal CEL source for a genuine
// buf.validate.field.cel/cel_expression rule, or — for a structured rule
// Tier 1 chooses not to translate (an overflow-guarded integer bound, a
// floating-point gt/lt) — a deterministic textual rendering of the
// rule's own value. Either way the fingerprint stays stable across runs
// and sensitive to a changed constraint (T-01-24), which is the property
// D-11/ANNO-02 actually need; only a genuine custom-CEL residual carries
// literal CEL source text.
type residualEntry struct {
	id   string
	expr string
}

// residualFingerprint is a hex-encoded SHA-256 over entries' expression
// text, sorted first so the result is byte-identical across repeated
// runs regardless of the order entries were collected in (D-24,
// TestResidualFingerprintStable). Returns "" for no entries, so a field
// with no residuals carries no fingerprint at all.
func residualFingerprint(entries []residualEntry) string {
	if len(entries) == 0 {
		return ""
	}
	exprs := make([]string, len(entries))
	for i, e := range entries {
		exprs[i] = e.expr
	}
	sort.Strings(exprs)
	h := sha256.New()
	for _, e := range exprs {
		h.Write([]byte(e))
		h.Write([]byte{0}) // NUL separator: never ambiguous with expression content
	}
	return hex.EncodeToString(h.Sum(nil))
}

// sortUnique returns ss deduplicated and sorted (D-24: annotation output
// must be byte-identical across runs).
func sortUnique(ss []string) []string {
	if len(ss) == 0 {
		return []string{}
	}
	set := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// celResidualsFrom extracts fd's message-independent custom CEL rules
// (buf.validate.field.cel and the simplified cel_expression form) as
// residual entries. Tier 1 never compiles or evaluates CEL (R1) — every
// custom expression is out of scope by construction and recorded as
// residual, sorted, fingerprinted text (D-11). rules is the value
// returned by protovalidate.ResolveFieldRules — passed straight through
// from the caller's local `:=` variable, never named by this file (see
// the file doc comment); accepting it as `any` and using it via a type
// switch would be another option, but since every call site already has
// the concrete methods in scope locally, this is instead implemented as
// a same-scope helper the caller inlines rather than a cross-function
// call, to avoid ever needing to spell the type. See mapScalar/
// mapOptionalScalar's StringKind and numeric cases (fieldmap.go).
func celRuleResidual(id, expression string) (residualID string, entry residualEntry) {
	if id == "" {
		id = expression
	}
	residualID = "cel." + id
	return residualID, residualEntry{id: residualID, expr: expression}
}

// --- String translation (VAL-01) ---------------------------------------

// applyStringConstraints translates sr's byte-semantic and code-point
// string bounds and its `pattern` rule onto sb, per the Task 1 decision
// (option-c: code-point bounds map directly onto ent's byte-comparing
// MinLen/MaxLen — a recorded divergence, not an oversight) and D-13
// (format validators, handled separately by applyStringFormat since it
// needs fd for the delegating path). Returns the updated builder, the
// translated constraint IDs, the (empty, in this function) residual IDs,
// the length-unit-divergent subset of translated (Task 1 resolution:
// "the divergence must be recorded... machine-visible"), and any
// residual entries (currently none from this function — pattern either
// translates or fails outright, per T-01-01's defensive-compile
// requirement).
func applyStringConstraints[T stringBuilderIface[T]](
	sb T, sr stringRulesIface,
) (out T, translated, divergent []string, err error) {
	out = sb

	// Byte-semantic bounds: exact on both sides under every Task 1
	// option (R5) — never divergent.
	if sr.HasMinBytes() {
		out = out.MinLen(int(sr.GetMinBytes()))
		translated = append(translated, "string.min_bytes")
	}
	if sr.HasMaxBytes() {
		out = out.MaxLen(int(sr.GetMaxBytes()))
		translated = append(translated, "string.max_bytes")
	}
	if sr.HasLenBytes() {
		n := int(sr.GetLenBytes())
		out = out.MinLen(n).MaxLen(n)
		translated = append(translated, "string.len_bytes")
	}

	// Code-point bounds: Task 1's resolved option-c — mapped directly
	// onto MinLen/MaxLen, and recorded as length-unit-divergent so the
	// divergence is machine-visible (Task 1 resolution), not just
	// documented in the README.
	if sr.HasMinLen() {
		out = out.MinLen(int(sr.GetMinLen()))
		translated = append(translated, "string.min_len")
		divergent = append(divergent, "string.min_len")
	}
	if sr.HasMaxLen() {
		out = out.MaxLen(int(sr.GetMaxLen()))
		translated = append(translated, "string.max_len")
		divergent = append(divergent, "string.max_len")
	}
	if sr.HasLen() {
		n := int(sr.GetLen())
		out = out.MinLen(n).MaxLen(n)
		translated = append(translated, "string.len")
		divergent = append(divergent, "string.len")
	}

	if sr.HasPattern() {
		re, cerr := regexp.Compile(sr.GetPattern())
		if cerr != nil {
			return out, translated, divergent, fmt.Errorf(
				"pattern %q fails to compile: %w", sr.GetPattern(), cerr,
			)
		}
		out = out.Match(re)
		translated = append(translated, "string.pattern")
	}

	return out, translated, divergent, nil
}

// protovalidateEmailPattern is copied byte-for-byte from protovalidate's
// own emailRegex (buf.build/go/protovalidate@v1.2.0, cel/library.go,
// unexported — verified against that source on 2026-08-08), not
// independently derived. D-13: where protovalidate's own semantics ARE a
// documented regular expression, Tier 1 emits Match with that same
// expression rather than a hand-written approximation.
const protovalidateEmailPattern = "^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$"

// applyStringFormat applies sr's well-known string format rule (if any)
// onto sb, per D-13's split:
//   - email is regex-defined (protovalidateEmailPattern above) — a
//     native Match() call, still fully introspectable.
//   - hostname/uri/ip/uuid are procedurally defined in protovalidate's
//     own evaluator (verified: no exported or literal-regex source was
//     found for them in buf.build/go/protovalidate@v1.2.0 this session —
//     uuid included, out of caution rather than risk a hand-approximated
//     regex that could diverge from protovalidate's own verdict) — each
//     delegates via .Validate(fn) to delegatingFormatValidator, which
//     invokes protovalidate's own real runtime evaluator rather than a
//     hand-written approximation, satisfying D-13's "delegating to
//     protovalidate's own predicate" literally: no new dependency is
//     needed since dynamicpb is already part of google.golang.org/
//     protobuf (one of the three direct dependencies).
//
// Returns ("", ...) with no error and no builder change when sr carries
// no WellKnown format rule at all, and ("string.well_known_unhandled",
// ...) as a residual constraint ID when sr.HasWellKnown() is true but
// none of the five D-13-scoped formats matched — some other well-known
// format (ipv4/ipv6/tuuid/host_and_port/...) is out of this plan's named
// scope and is recorded, not silently dropped (T-01-23).
func applyStringFormat[T stringBuilderIface[T]](
	sb T, fd protoreflect.FieldDescriptor, sr stringRulesIface,
) (out T, translated string, residual string, entry *residualEntry, err error) {
	out = sb
	if !sr.HasWellKnown() {
		return out, "", "", nil, nil
	}

	switch {
	case sr.GetEmail():
		// regexp.Compile, not MustCompile, even though
		// protovalidateEmailPattern is mixinforproto's own hardcoded,
		// TestStringFormatValidators-proven constant (never
		// contract-supplied, so T-01-01's ReDoS/malformed-pattern threat
		// model does not attach here) — kept symmetric with the
		// contract-supplied pattern path above for one uniform,
		// grep-able "no MustCompile in this file" invariant.
		re, cerr := regexp.Compile(protovalidateEmailPattern)
		if cerr != nil {
			return out, "", "", nil, fmt.Errorf("compiling mixinforproto's own email pattern (this is a bug, not a contract error): %w", cerr)
		}
		return out.Match(re), "string.email", "", nil, nil
	case sr.GetHostname():
		fn, ferr := delegatingFormatValidator(fd, "hostname")
		if ferr != nil {
			return out, "", "", nil, ferr
		}
		return out.Validate(fn), "string.hostname", "", nil, nil
	case sr.GetUri():
		fn, ferr := delegatingFormatValidator(fd, "uri")
		if ferr != nil {
			return out, "", "", nil, ferr
		}
		return out.Validate(fn), "string.uri", "", nil, nil
	case sr.GetIp():
		fn, ferr := delegatingFormatValidator(fd, "ip")
		if ferr != nil {
			return out, "", "", nil, ferr
		}
		return out.Validate(fn), "string.ip", "", nil, nil
	case sr.GetUuid():
		fn, ferr := delegatingFormatValidator(fd, "uuid")
		if ferr != nil {
			return out, "", "", nil, ferr
		}
		return out.Validate(fn), "string.uuid", "", nil, nil
	default:
		id := "string.well_known_unhandled"
		e := residualEntry{id: id, expr: id + " (format outside D-13's five-format scope; recorded, not enforced)"}
		return out, "", id, &e, nil
	}
}

// delegatingFormatValidator returns a func(string) error that delegates
// a procedurally-defined protovalidate string format rule to
// protovalidate's own runtime evaluator, rather than a hand-written
// regex approximation (D-13). It builds a real protovalidate.Validator
// once, at schema-derivation time (never per-call), and the returned
// closure constructs a synthetic dynamicpb.Message carrying only the
// candidate value for fd, then calls protovalidate's own Validate on
// that WHOLE message.
//
// That whole-message call can surface violations for fields OTHER than
// fd (every other field on the synthetic message sits at its zero
// value, so an unrelated `required` or any other rule can produce its
// own violation). The mechanism this function actually relies on is NOT
// "the corpus keeps format-validator messages single-field" — an
// earlier version of this comment claimed that, and it was wrong
// (01-VERIFICATION.md gap 2 / 01-REVIEW.md CR-02: on any multi-field
// message it rejected effectively 100% of inputs, valid ones included).
// The real mechanism is per-field verdict extraction: on a non-nil
// error, the violations are filtered down to the one (if any) whose
// FieldDescriptor names fd, by FullName comparison. A violation on any
// OTHER field is discarded — it is not this field's verdict. A
// violation ON fd is still, and must remain, a rejection: this half of
// the contract is what stops the filter from becoming a validation
// bypass (T-01G-10) — see TestDelegatedFormatIgnoresUnrelatedViolations
// for both assertions exercised together. The verdict returned for fd
// is still protovalidate's own, not a lossy reimplementation — this is
// what makes it "delegating to protovalidate's own predicate" in fact,
// not just in name.
func delegatingFormatValidator(fd protoreflect.FieldDescriptor, formatName string) (func(string) error, error) {
	v, err := protovalidate.New()
	if err != nil {
		return nil, fmt.Errorf("building protovalidate validator for %s format delegation: %w", formatName, err)
	}
	md := fd.ContainingMessage()
	fieldName := fd.FullName()
	return func(s string) error {
		msg := dynamicpb.NewMessage(md)
		msg.Set(fd, protoreflect.ValueOfString(s))
		verr := v.Validate(msg)
		if verr == nil {
			return nil
		}
		var ve *protovalidate.ValidationError
		if !errors.As(verr, &ve) {
			// Not a rule-violation verdict at all — a genuine evaluator
			// failure (e.g. a CEL evaluation error). Surface it as such
			// rather than masquerading as "invalid value".
			return fmt.Errorf("evaluating the %s format constraint: %w", formatName, verr)
		}
		for _, viol := range ve.Violations {
			if viol.FieldDescriptor != nil && viol.FieldDescriptor.FullName() == fieldName {
				return fmt.Errorf("value does not satisfy the %s format constraint", formatName)
			}
		}
		// Every violation belonged to some other field on the synthetic
		// message — not this field's business.
		return nil
	}, nil
}

// --- Numeric translation (VAL-02) ---------------------------------------

// applySignedRange translates a signed-integer numeric Rules message's
// gt/gte/lt/lte onto sb per D-12: gte/lte translate unchanged; gt/lt
// adjust exactly (+1/-1) UNLESS the adjustment would overflow the
// field's own type range (gt at the type maximum, lt at the type
// minimum), in which case the constraint is recorded residual instead of
// silently inverting the bound (T-01-22). A lower and upper bound
// together collapse into a single Range call over the adjusted values,
// emitted faithfully even when the adjusted bounds coincide or cross
// (Tier 1's contract is verdict identity with protovalidate, not
// contract review). typeMin/typeMax are the field's own proto integer
// type's bounds, widened to int64 — exact and lossless for every signedNum
// this plan translates (int32, int64).
func applySignedRange[V signedNum, T numBuilderIface[T, V]](
	sb T, r rangeRulesIface[V], kindName string, typeMin, typeMax int64,
) (out T, translated, residual []string, entries []residualEntry) {
	out = sb
	var lower, upper V
	var hasLower, hasUpper bool

	switch {
	case r.HasGte():
		lower, hasLower = r.GetGte(), true
		translated = append(translated, kindName+".gte")
	case r.HasGt():
		n := int64(r.GetGt())
		if n >= typeMax {
			residual = append(residual, kindName+".gt")
			entries = append(entries, residualEntry{
				id:   kindName + ".gt",
				expr: fmt.Sprintf("%s.gt=%d (adjustment to %d would overflow this field's type range)", kindName, n, n+1),
			})
		} else {
			lower, hasLower = V(n+1), true
			translated = append(translated, kindName+".gt")
		}
	}

	switch {
	case r.HasLte():
		upper, hasUpper = r.GetLte(), true
		translated = append(translated, kindName+".lte")
	case r.HasLt():
		n := int64(r.GetLt())
		if n <= typeMin {
			residual = append(residual, kindName+".lt")
			entries = append(entries, residualEntry{
				id:   kindName + ".lt",
				expr: fmt.Sprintf("%s.lt=%d (adjustment to %d would overflow this field's type range)", kindName, n, n-1),
			})
		} else {
			upper, hasUpper = V(n-1), true
			translated = append(translated, kindName+".lt")
		}
	}

	switch {
	case hasLower && hasUpper:
		out = out.Range(lower, upper)
	case hasLower:
		out = out.Min(lower)
	case hasUpper:
		out = out.Max(upper)
	}
	return out, translated, residual, entries
}

// applyFloatRange translates a floating-point numeric Rules message's
// gte/lte onto sb exactly (no adjustment needed) while ALWAYS recording
// gt/lt as residual, never widening them to gte/lte (D-12): widening
// would make the schema layer accept a value protovalidate rejects, and
// Phase 3's differential harness exists precisely to catch that class of
// disagreement.
func applyFloatRange[V floatNum, T numBuilderIface[T, V]](
	sb T, r rangeRulesIface[V], kindName string,
) (out T, translated, residual []string, entries []residualEntry) {
	out = sb
	var lower, upper V
	var hasLower, hasUpper bool

	switch {
	case r.HasGte():
		lower, hasLower = r.GetGte(), true
		translated = append(translated, kindName+".gte")
	case r.HasGt():
		residual = append(residual, kindName+".gt")
		entries = append(entries, residualEntry{
			id:   kindName + ".gt",
			expr: fmt.Sprintf("%s.gt=%v (floating-point gt is never translated — D-12)", kindName, r.GetGt()),
		})
	}

	switch {
	case r.HasLte():
		upper, hasUpper = r.GetLte(), true
		translated = append(translated, kindName+".lte")
	case r.HasLt():
		residual = append(residual, kindName+".lt")
		entries = append(entries, residualEntry{
			id:   kindName + ".lt",
			expr: fmt.Sprintf("%s.lt=%v (floating-point lt is never translated — D-12)", kindName, r.GetLt()),
		})
	}

	switch {
	case hasLower && hasUpper:
		out = out.Range(lower, upper)
	case hasLower:
		out = out.Min(lower)
	case hasUpper:
		out = out.Max(upper)
	}
	return out, translated, residual, entries
}

// recordAnyRangeResidual records whichever of gt/gte/lt/lte are present
// on r as residual constraint IDs under kindName — used for the numeric
// kinds this plan does not translate (unsigned integers: uint32/uint64/
// fixed32/fixed64; see 01-05-SUMMARY.md's documented scope boundary) so
// a real constraint is recorded, never silently dropped (D-11/T-01-23),
// even though this plan's corpus does not exercise them.
func recordAnyRangeResidual(r anyRangeRulesPresence, kindName string) (residual []string, entries []residualEntry) {
	add := func(suffix string) {
		id := kindName + "." + suffix
		residual = append(residual, id)
		entries = append(entries, residualEntry{
			id:   id,
			expr: id + " (unsigned interval translation is out of this plan's scope; recorded, not enforced)",
		})
	}
	if r.HasGt() {
		add("gt")
	}
	if r.HasGte() {
		add("gte")
	}
	if r.HasLt() {
		add("lt")
	}
	if r.HasLte() {
		add("lte")
	}
	return residual, entries
}

// bytesRulesPresence is satisfied by *validate.BytesRules. bytes.min_len/
// max_len/len/pattern translation is out of this plan's scope (VAL-01
// names "string" specifically) — recordBytesResidual below uses this
// presence-only projection so a real bytes.* constraint is still
// recorded, never silently dropped (D-11/T-01-23), even though it is not
// yet translated exactly.
type bytesRulesPresence interface {
	HasMinLen() bool
	HasMaxLen() bool
	HasLen() bool
	HasPattern() bool
}

// recordBytesResidual records whichever of min_len/max_len/len/pattern
// are present on r as residual constraint IDs under "bytes.*" — see
// bytesRulesPresence's doc comment.
func recordBytesResidual(r bytesRulesPresence) (residual []string, entries []residualEntry) {
	add := func(suffix string) {
		id := "bytes." + suffix
		residual = append(residual, id)
		entries = append(entries, residualEntry{
			id:   id,
			expr: id + " (bytes constraint translation is out of this plan's scope — VAL-01 names string specifically; recorded, not enforced)",
		})
	}
	if r.HasMinLen() {
		add("min_len")
	}
	if r.HasMaxLen() {
		add("max_len")
	}
	if r.HasLen() {
		add("len")
	}
	if r.HasPattern() {
		add("pattern")
	}
	return residual, entries
}
