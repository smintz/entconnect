package mixinforproto

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/reflect/protoreflect"

	"buf.build/go/protovalidate"
)

// fieldClass is the classification a proto field descriptor is sorted
// into before mapping. classify's branch order is load-bearing (see
// classify's doc comment) — getting it wrong is precisely what would
// make every proto3 `optional` scalar panic, per 01-RESEARCH.md's
// synthetic-oneof finding.
type fieldClass int

const (
	classScalarMap fieldClass = iota
	classMessageMap
	classRealOneofMember
	classOptionalScalar
	classMessageField
	classEnum
	classScalar
	// classRepeated is appended at the end of the iota run (01-06-PLAN.md
	// gap closure) so every pre-existing value keeps its numbering — the
	// code review's reproduction and 01-09's corpus-adequacy guard both
	// cite the prior numeric values verbatim.
	classRepeated
)

// classify sorts fd into exactly one fieldClass, in the exact order
// 01-RESEARCH.md verified: IsMap() first; then IsList() (01-06-PLAN.md
// gap closure, MIX-02/CR-01); then real-oneof membership
// (ContainingOneof() != nil && !ContainingOneof().IsSynthetic()); then
// HasOptionalKeyword(); then MessageKind; then EnumKind; then plain
// scalar. The order matters because the branches overlap:
//   - IsMap() MUST be checked before IsList(): in protoreflect a map
//     field also reports IsList() == true, so inverting these two would
//     silently reclassify every map field as classRepeated instead of
//     classScalarMap/classMessageMap — this exact ordering hazard is
//     T-01G-01 in this plan's threat model.
//   - a proto3 `optional` scalar is wrapped by the compiler in a
//     synthetic one-member oneof, so ContainingOneof() is non-nil for it
//     too — only IsSynthetic() (checked as part of the real-oneof
//     branch, ahead of the optional-keyword branch) separates MIX-05
//     from MIX-10.
//
// A field cannot satisfy both of any overlapping pair above.
func classify(fd protoreflect.FieldDescriptor) fieldClass {
	switch {
	case fd.IsMap():
		if fd.MapValue().Kind() == protoreflect.MessageKind {
			return classMessageMap
		}
		return classScalarMap
	case fd.IsList():
		return classRepeated
	case fd.ContainingOneof() != nil && !fd.ContainingOneof().IsSynthetic():
		return classRealOneofMember
	case fd.HasOptionalKeyword():
		return classOptionalScalar
	case fd.Kind() == protoreflect.MessageKind:
		return classMessageField
	case fd.Kind() == protoreflect.EnumKind:
		return classEnum
	default:
		return classScalar
	}
}

// Well-known-type full names (MIX-04). These are protobuf's own fixed,
// unversioned type names — identification is a string comparison
// against fd.Message().FullName(), the standard protobuf-go idiom.
const (
	wktTimestamp = "google.protobuf.Timestamp"
	wktStruct    = "google.protobuf.Struct"
	wktValue     = "google.protobuf.Value"
	wktFieldMask = "google.protobuf.FieldMask"
	wktDuration  = "google.protobuf.Duration"
)

// mapField translates one proto field descriptor into a derived
// ent.Field, or a self-sufficient error naming the message, the field,
// what is unsupported, and the fix (D-08, MIX-11). A nil, nil return
// means the field is deliberately skipped (a real-oneof member left
// unresolved by this plan, a message-valued map, an un-opted-in message
// field, a skipped well-known type, or an un-opted-in repeated message
// field — MIX-09/MIX-10, unchanged by 01-06-PLAN.md's gap closure) —
// not a silent failure.
//
// o carries the effective Option set for this derivation (Exclude,
// Override, AsJSON) so AsJSON's message-field opt-in (MIX-09) can be
// checked per field; derive.go's existing o variable is threaded
// through the one call site unchanged in every other respect.
func mapField(msgName string, fd protoreflect.FieldDescriptor, o *options) (ent.Field, error) {
	name := string(fd.Name())

	switch classify(fd) {
	case classScalarMap:
		return mapScalarMap(fd), nil
	case classMessageMap:
		// MIX-06: message-valued maps are skipped, not an error.
		return nil, nil
	case classRealOneofMember:
		// MIX-10: this plan classifies real-oneof members only. It
		// produces no field and records no error for them — the
		// panic-unless-all-resolved gate over a oneof's members is
		// Plan 03's job and lands in derive.go, not here.
		return nil, nil
	case classOptionalScalar:
		return mapOptionalScalar(msgName, name, fd)
	case classMessageField:
		return mapMessageField(fd, o)
	case classEnum:
		return mapEnum(fd), nil
	case classRepeated:
		return mapRepeated(fd, o)
	default: // classScalar
		return mapScalar(msgName, name, fd)
	}
}

// mapRepeated handles the classRepeated branch (01-06-PLAN.md, closing
// 01-VERIFICATION.md gap 1 / CR-01): a `repeated` field never derives a
// scalar/enum ent field in v0.1 — that would silently drop the
// contract's cardinality, and worse, could build a validator closure
// that panics at ent mutation time against a list-cardinality
// descriptor (T-01G-02). A repeated MESSAGE-typed field is the one
// exception: it delegates to mapMessageField, preserving MIX-09's
// AsJSON opt-in and MIX-10's skip-by-default exactly as before
// (mapWellKnownType already returns handled=false for lists, so a
// repeated well-known type keeps falling through to the same skip).
// Every other repeated kind returns a plain error; derive.go's existing
// mapField-error wrapper turns it into a failure{rule:"fieldmap"} with
// the standard Exclude/Override remedy (D-08/D-09), and MIX-12's
// Validate[M] surfaces the identical message — no new error machinery.
func mapRepeated(fd protoreflect.FieldDescriptor, o *options) (ent.Field, error) {
	if fd.Kind() == protoreflect.MessageKind {
		return mapMessageField(fd, o)
	}
	return nil, fmt.Errorf(
		"repeated field has no supported ent mapping in v0.1 (element kind %s) — cardinality would be silently dropped",
		fd.Kind(),
	)
}

// sourceFieldFor builds the SourceField annotation every derived field
// carries (D-02/ANNO-02), with Kind set to the derivation class name
// ("scalar" / "optionalScalar" / "enum" / "wkt" / "scalarMap" /
// "asJSON") rather than the raw protoreflect.Kind string — this is the
// "derivation kind (scalar / enum / WKT / map-as-JSON / message-as-JSON
// / override)" D-02 documents. TranslatedIDs/ResidualIDs stay empty:
// Tier 1 translation is Plan 04's job.
func sourceFieldFor(fd protoreflect.FieldDescriptor, class string) SourceField {
	return SourceField{
		ContractVersion:        ContractVersion,
		FieldName:              string(fd.Name()),
		Number:                 int32(fd.Number()),
		Kind:                   class,
		TranslatedIDs:          []string{},
		ResidualIDs:            []string{},
		LengthUnitDivergentIDs: []string{},
	}
}

// finalizeSourceField builds the SourceField annotation for a field Tier
// 1 actually translated constraints for (string/numeric scalars):
// translated/residual/divergent are deduplicated and sorted (D-24) and
// entries feed the residual fingerprint (T-01-24). Kinds Tier 1 does not
// translate (enum, WKT, scalar-map, AsJSON, message-typed) keep using
// plain sourceFieldFor above, with empty (not absent) ID lists.
func finalizeSourceField(
	fd protoreflect.FieldDescriptor, class string,
	translated, residual, divergent []string, entries []residualEntry,
) SourceField {
	return SourceField{
		ContractVersion:        ContractVersion,
		FieldName:              string(fd.Name()),
		Number:                 int32(fd.Number()),
		Kind:                   class,
		TranslatedIDs:          sortUnique(translated),
		ResidualIDs:            sortUnique(residual),
		LengthUnitDivergentIDs: sortUnique(divergent),
		ResidualFingerprint:    residualFingerprint(entries),
	}
}

// resolvedFieldRules resolves fd's protovalidate FieldRules, its CEL
// residual entries (custom (buf.validate.field).cel/cel_expression rules
// — Tier 1 never compiles or evaluates CEL, R1), and its `required` flag,
// all in one place so every scalar-kind case below (mapScalar,
// mapOptionalScalar) shares the same nil-safe resolution and the same
// CEL-residual extraction rather than duplicating either per Kind case.
// A constraint-free field yields a nil rules value (ResolveFieldRules's
// verified (nil, nil) return, Plan 01) — celResidual/celEntries/required
// are all zero-valued in that case, never dereferenced.
func resolvedFieldRules(fd protoreflect.FieldDescriptor) (
	required bool, celResidual []string, celEntries []residualEntry, err error,
) {
	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil {
		return false, nil, nil, fmt.Errorf("resolving protovalidate field rules: %w", err)
	}
	if rules == nil {
		return false, nil, nil, nil
	}
	required = rules.HasRequired() && rules.GetRequired()
	for _, r := range rules.GetCel() {
		id, entry := celRuleResidual(r.GetId(), r.GetExpression())
		celResidual = append(celResidual, id)
		celEntries = append(celEntries, entry)
	}
	for _, expr := range rules.GetCelExpression() {
		id, entry := celRuleResidual("", expr)
		celResidual = append(celResidual, id)
		celEntries = append(celEntries, entry)
	}
	return required, celResidual, celEntries, nil
}

// boundaryOnlyNonConstraintFields are validate.FieldRules members that
// are bookkeeping, not a constraint value in their own right — Range
// below skips them because "required" and the CEL rules are already
// recorded by name/ID above, and "ignore" only modifies when a sibling
// constraint applies rather than declaring one itself.
var boundaryOnlyNonConstraintFields = map[string]bool{
	"required":       true,
	"ignore":         true,
	"cel":            true,
	"cel_expression": true,
}

// boundaryOnlyRuleIDs resolves the protovalidate constraint identifiers
// present on fd generically — not translated, not evaluated, just
// enumerated — for D-09's boundary-only provenance (annotation.go's
// BoundaryOnlyRule). Unlike resolvedFieldRules above (called only from
// the scalar-kind builders, and only for the standard rule categories
// those specific builders translate), this works for ANY field kind and
// ANY rule category, including a message-typed field that never reaches
// a builder function at all: the fields recordBoundaryOnly (derive.go)
// calls this for are exactly the ones classify/mapField routed away from
// every buildXxxField call site, so there is no per-kind builder to reuse
// here.
//
// "required" and each (buf.validate.field).cel rule's ID (celRuleResidual's
// own ID-or-expression-fingerprint fallback) are recorded by name/ID.
// Every other populated rule category — validate.FieldRules' `type`
// oneof (string/int32/enum/bytes/map/...) — is recorded generically by
// its oneof member name via a single protoreflect.Message.Range pass:
// this function's job is enumeration for provenance, not translation, so
// a category name like "string" is sufficient; the rule is never
// evaluated or compared against a specific constraint value here. Result
// is sorted and deduplicated (D-24); a constraint-free field or a nil
// rules value yields an empty, non-nil slice.
func boundaryOnlyRuleIDs(fd protoreflect.FieldDescriptor) ([]string, error) {
	rules, err := protovalidate.ResolveFieldRules(fd)
	if err != nil {
		return nil, fmt.Errorf("resolving protovalidate field rules: %w", err)
	}
	if rules == nil {
		return []string{}, nil
	}
	var ids []string
	if rules.HasRequired() && rules.GetRequired() {
		ids = append(ids, "required")
	}
	for _, r := range rules.GetCel() {
		id, _ := celRuleResidual(r.GetId(), r.GetExpression())
		ids = append(ids, id)
	}
	for _, expr := range rules.GetCelExpression() {
		id, _ := celRuleResidual("", expr)
		ids = append(ids, id)
	}
	rules.ProtoReflect().Range(func(rfd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if name := string(rfd.Name()); !boundaryOnlyNonConstraintFields[name] {
			ids = append(ids, name)
		}
		return true
	})
	return sortUnique(ids), nil
}

// reverseBindableKinds is the set of SourceField.Kind derivation-class
// strings reverse.go's reverseValue can convert. Used only by
// recordBoundaryOnly's BoundaryOnlyUnbindable case (derive.go) — see that
// constant's doc comment (annotation.go) for why this branch is dormant
// today.
var reverseBindableKinds = map[string]bool{
	"scalar":         true,
	"optionalScalar": true,
	"enum":           true,
	"wkt":            true,
	"scalarMap":      true,
	"asJSON":         true,
}

// sourceFieldKind extracts the Kind string of f's SourceField annotation
// (fieldmap.go's sourceFieldFor/finalizeSourceField attach one to every
// field mapField produces), or "" if f carries none — the only field that
// never does is an Override-supplied field, which installs verbatim with
// no SourceField provenance (option.go's Override doc comment) and is
// never passed to this function anyway (Override's own boundary-only
// recording uses BoundaryOnlyOverridden directly, never this path).
func sourceFieldKind(f ent.Field) string {
	for _, a := range f.Descriptor().Annotations {
		if sf, ok := a.(SourceField); ok {
			return sf.Kind
		}
	}
	return ""
}

// mapScalar handles MIX-02 (the exhaustive proto-scalar-kind switch) and
// MIX-05's non-optional branch, now layering Tier 1 constraint
// translation (01-05-PLAN.md) on top: a plain proto3 scalar without
// `required` still gets .Default(<Go zero of its type>) and stays
// non-optional, matching the wire's own collapse of unset and zero
// (D-26 documents the consequence). No widening: each kind maps to its
// same-width ent builder, including the sint/fixed/sfixed variants of
// the same width. Delegates to the buildXxxField(name, fd, optional)
// functions below, shared with mapOptionalScalar (optional=false here).
func mapScalar(msgName, name string, fd protoreflect.FieldDescriptor) (ent.Field, error) {
	return buildScalarField(name, fd, false)
}

// mapOptionalScalar handles MIX-05's optional-keyword branch: a proto3
// `optional` scalar gets .Nillable().Optional() and no default —
// HasOptionalKeyword() being true here (verified by classify's caller,
// classOptionalScalar) is exactly the presence signal that means the
// wire distinguishes unset from zero, so nothing may be collapsed —
// UNLESS `required` is also set (VAL-03), in which case the exact
// translation drops Nillable/Optional/Default entirely rather than
// approximating presence (see buildXxxField's required-handling below).
func mapOptionalScalar(msgName, name string, fd protoreflect.FieldDescriptor) (ent.Field, error) {
	return buildScalarField(name, fd, true)
}

// buildScalarField dispatches to the per-Go-type builder function
// shared between mapScalar (optional=false) and mapOptionalScalar
// (optional=true) — the single place both MIX-05 branches and every
// Tier 1 translation path (VAL-01/VAL-02/VAL-03) meet.
func buildScalarField(name string, fd protoreflect.FieldDescriptor, optional bool) (ent.Field, error) {
	switch fd.Kind() {
	case protoreflect.DoubleKind:
		return buildFloat64Field(name, fd, optional)
	case protoreflect.FloatKind:
		return buildFloat32Field(name, fd, optional)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return buildInt32Field(name, fd, optional)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return buildInt64Field(name, fd, optional)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return buildUint32Field(name, fd, optional)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return buildUint64Field(name, fd, optional)
	case protoreflect.BoolKind:
		return buildBoolField(name, fd, optional)
	case protoreflect.StringKind:
		return buildStringField(name, fd, optional)
	case protoreflect.BytesKind:
		return buildBytesField(name, fd, optional)
	default:
		kindLabel := "field"
		if optional {
			kindLabel = "optional field"
		}
		return nil, fmt.Errorf("unsupported %s kind %q — this mapping rule lands in a later plan", kindLabel, fd.Kind())
	}
}

// scalarClass returns the SourceField.Kind value for a scalar
// derivation, matching D-02's documented class names.
func scalarClass(optional bool) string {
	if optional {
		return "optionalScalar"
	}
	return "scalar"
}

// requiredResult carries the three MECE outcomes VAL-03's "required"
// translation can take for a scalar field, computed once and applied
// identically across every buildXxxField function below:
//   - optional && required: exact for EVERY kind — non-optional
//     construction (no Nillable/Optional/Default), matching
//     protovalidate's "must be set" semantics for a presence-tracking
//     field. This case wins ahead of the NotEmpty case below: on a
//     presence-tracking field, `required` is a presence assertion, never
//     a non-emptiness assertion (01-VERIFICATION.md gap 3 /
//     01-REVIEW.md CR-03 — a set-but-empty string is valid to
//     protovalidate here, and NotEmpty() wrongly rejected it before this
//     branch order was corrected).
//   - required && hasNotEmpty (implicit presence: no `optional`
//     keyword): exact via NotEmpty() — protovalidate's "can't be the
//     zero value" semantics for a non-presence-tracking string/bytes
//     field.
//   - optional && !required: MIX-05's ordinary Nillable().Optional()
//     branch, untouched by Tier 1.
//   - !optional && required (implicit presence, no NotEmpty-equivalent
//     for this kind): NOT exact for scalars without a typed
//     .NotEmpty()-equivalent (VAL-03's flagged ambiguity) — recorded
//     residual, never approximated.
//   - !optional && !required: MIX-05's ordinary Default(zero) branch.
type requiredResult int

const (
	requiredNone          requiredResult = iota // !optional && !required
	requiredExactPresence                       // optional && required (presence wins, every kind)
	requiredExactNotEmpty                       // required, string/bytes, implicit presence only
	requiredResidualZero                        // !optional && required, no exact translation
	requiredOptionalPlain                       // optional && !required
)

func classifyRequired(optional, required, hasNotEmpty bool) requiredResult {
	switch {
	case optional && required:
		return requiredExactPresence
	case required && hasNotEmpty:
		return requiredExactNotEmpty
	case optional:
		return requiredOptionalPlain
	case required:
		return requiredResidualZero
	default:
		return requiredNone
	}
}

// buildInt32Field builds the derived field for Int32Kind/Sint32Kind/
// Sfixed32Kind, sharing D-12's overflow-guarded signed-range translation
// between mapScalar's plain path and mapOptionalScalar's `optional`
// path via the optional parameter.
func buildInt32Field(name string, fd protoreflect.FieldDescriptor, optional bool) (ent.Field, error) {
	required, celResidual, celEntries, err := resolvedFieldRules(fd)
	if err != nil {
		return nil, err
	}
	sb := field.Int32(name)
	var translated, residual []string
	var entries []residualEntry

	rules, rerr := protovalidate.ResolveFieldRules(fd)
	if rerr != nil {
		return nil, fmt.Errorf("resolving protovalidate field rules: %w", rerr)
	}
	if rules != nil {
		switch {
		case rules.HasInt32():
			sb, translated, residual, entries = applySignedRange[int32](sb, rules.GetInt32(), "int32", math.MinInt32, math.MaxInt32)
		case rules.HasSint32():
			sb, translated, residual, entries = applySignedRange[int32](sb, rules.GetSint32(), "sint32", math.MinInt32, math.MaxInt32)
		case rules.HasSfixed32():
			sb, translated, residual, entries = applySignedRange[int32](sb, rules.GetSfixed32(), "sfixed32", math.MinInt32, math.MaxInt32)
		}
	}
	residual = append(residual, celResidual...)
	entries = append(entries, celEntries...)

	switch classifyRequired(optional, required, false) {
	case requiredExactPresence:
		translated = append(translated, "required")
	case requiredOptionalPlain:
		sb = sb.Nillable().Optional()
	case requiredResidualZero:
		residual = append(residual, "required")
		sb = sb.Default(0)
	default: // requiredNone
		sb = sb.Default(0)
	}

	sf := finalizeSourceField(fd, scalarClass(optional), translated, residual, nil, entries)
	return sb.Annotations(sf), nil
}

// buildInt64Field mirrors buildInt32Field for Int64Kind/Sint64Kind/
// Sfixed64Kind.
func buildInt64Field(name string, fd protoreflect.FieldDescriptor, optional bool) (ent.Field, error) {
	required, celResidual, celEntries, err := resolvedFieldRules(fd)
	if err != nil {
		return nil, err
	}
	sb := field.Int64(name)
	var translated, residual []string
	var entries []residualEntry

	rules, rerr := protovalidate.ResolveFieldRules(fd)
	if rerr != nil {
		return nil, fmt.Errorf("resolving protovalidate field rules: %w", rerr)
	}
	if rules != nil {
		switch {
		case rules.HasInt64():
			sb, translated, residual, entries = applySignedRange[int64](sb, rules.GetInt64(), "int64", math.MinInt64, math.MaxInt64)
		case rules.HasSint64():
			sb, translated, residual, entries = applySignedRange[int64](sb, rules.GetSint64(), "sint64", math.MinInt64, math.MaxInt64)
		case rules.HasSfixed64():
			sb, translated, residual, entries = applySignedRange[int64](sb, rules.GetSfixed64(), "sfixed64", math.MinInt64, math.MaxInt64)
		}
	}
	residual = append(residual, celResidual...)
	entries = append(entries, celEntries...)

	switch classifyRequired(optional, required, false) {
	case requiredExactPresence:
		translated = append(translated, "required")
	case requiredOptionalPlain:
		sb = sb.Nillable().Optional()
	case requiredResidualZero:
		residual = append(residual, "required")
		sb = sb.Default(0)
	default:
		sb = sb.Default(0)
	}

	sf := finalizeSourceField(fd, scalarClass(optional), translated, residual, nil, entries)
	return sb.Annotations(sf), nil
}

// buildUint32Field builds Uint32Kind/Fixed32Kind. Unsigned interval
// constraints (gt/gte/lt/lte on uint32/fixed32) are recorded as residual
// rather than translated in this plan (recordAnyRangeResidual,
// validate.go) — a deliberate, documented scope boundary (01-05-
// SUMMARY.md): this plan's corpus exercises signed-integer and
// floating-point interval translation only (Task 3's action text), and
// recording-not-dropping an unsigned bound is still exactly correct per
// D-11/T-01-23, just not yet exact.
func buildUint32Field(name string, fd protoreflect.FieldDescriptor, optional bool) (ent.Field, error) {
	required, celResidual, celEntries, err := resolvedFieldRules(fd)
	if err != nil {
		return nil, err
	}
	sb := field.Uint32(name)
	var translated, residual []string
	var entries []residualEntry

	rules, rerr := protovalidate.ResolveFieldRules(fd)
	if rerr != nil {
		return nil, fmt.Errorf("resolving protovalidate field rules: %w", rerr)
	}
	if rules != nil {
		switch {
		case rules.HasUint32():
			r, e := recordAnyRangeResidual(rules.GetUint32(), "uint32")
			residual = append(residual, r...)
			entries = append(entries, e...)
		case rules.HasFixed32():
			r, e := recordAnyRangeResidual(rules.GetFixed32(), "fixed32")
			residual = append(residual, r...)
			entries = append(entries, e...)
		}
	}
	residual = append(residual, celResidual...)
	entries = append(entries, celEntries...)

	switch classifyRequired(optional, required, false) {
	case requiredExactPresence:
		translated = append(translated, "required")
	case requiredOptionalPlain:
		sb = sb.Nillable().Optional()
	case requiredResidualZero:
		residual = append(residual, "required")
		sb = sb.Default(0)
	default:
		sb = sb.Default(0)
	}

	sf := finalizeSourceField(fd, scalarClass(optional), translated, residual, nil, entries)
	return sb.Annotations(sf), nil
}

// buildUint64Field mirrors buildUint32Field for Uint64Kind/Fixed64Kind.
func buildUint64Field(name string, fd protoreflect.FieldDescriptor, optional bool) (ent.Field, error) {
	required, celResidual, celEntries, err := resolvedFieldRules(fd)
	if err != nil {
		return nil, err
	}
	sb := field.Uint64(name)
	var translated, residual []string
	var entries []residualEntry

	rules, rerr := protovalidate.ResolveFieldRules(fd)
	if rerr != nil {
		return nil, fmt.Errorf("resolving protovalidate field rules: %w", rerr)
	}
	if rules != nil {
		switch {
		case rules.HasUint64():
			r, e := recordAnyRangeResidual(rules.GetUint64(), "uint64")
			residual = append(residual, r...)
			entries = append(entries, e...)
		case rules.HasFixed64():
			r, e := recordAnyRangeResidual(rules.GetFixed64(), "fixed64")
			residual = append(residual, r...)
			entries = append(entries, e...)
		}
	}
	residual = append(residual, celResidual...)
	entries = append(entries, celEntries...)

	switch classifyRequired(optional, required, false) {
	case requiredExactPresence:
		translated = append(translated, "required")
	case requiredOptionalPlain:
		sb = sb.Nillable().Optional()
	case requiredResidualZero:
		residual = append(residual, "required")
		sb = sb.Default(0)
	default:
		sb = sb.Default(0)
	}

	sf := finalizeSourceField(fd, scalarClass(optional), translated, residual, nil, entries)
	return sb.Annotations(sf), nil
}

// buildFloat32Field builds FloatKind, applying D-12's floating-point
// posture: gt/lt are always residual, never widened to gte/lte; gte/lte
// translate exactly.
func buildFloat32Field(name string, fd protoreflect.FieldDescriptor, optional bool) (ent.Field, error) {
	required, celResidual, celEntries, err := resolvedFieldRules(fd)
	if err != nil {
		return nil, err
	}
	sb := field.Float32(name)
	var translated, residual []string
	var entries []residualEntry

	rules, rerr := protovalidate.ResolveFieldRules(fd)
	if rerr != nil {
		return nil, fmt.Errorf("resolving protovalidate field rules: %w", rerr)
	}
	if rules != nil && rules.HasFloat() {
		sb, translated, residual, entries = applyFloatRange[float32](sb, rules.GetFloat(), "float")
	}
	residual = append(residual, celResidual...)
	entries = append(entries, celEntries...)

	switch classifyRequired(optional, required, false) {
	case requiredExactPresence:
		translated = append(translated, "required")
	case requiredOptionalPlain:
		sb = sb.Nillable().Optional()
	case requiredResidualZero:
		residual = append(residual, "required")
		sb = sb.Default(0)
	default:
		sb = sb.Default(0)
	}

	sf := finalizeSourceField(fd, scalarClass(optional), translated, residual, nil, entries)
	return sb.Annotations(sf), nil
}

// buildFloat64Field mirrors buildFloat32Field for DoubleKind.
func buildFloat64Field(name string, fd protoreflect.FieldDescriptor, optional bool) (ent.Field, error) {
	required, celResidual, celEntries, err := resolvedFieldRules(fd)
	if err != nil {
		return nil, err
	}
	sb := field.Float(name)
	var translated, residual []string
	var entries []residualEntry

	rules, rerr := protovalidate.ResolveFieldRules(fd)
	if rerr != nil {
		return nil, fmt.Errorf("resolving protovalidate field rules: %w", rerr)
	}
	if rules != nil && rules.HasDouble() {
		sb, translated, residual, entries = applyFloatRange[float64](sb, rules.GetDouble(), "double")
	}
	residual = append(residual, celResidual...)
	entries = append(entries, celEntries...)

	switch classifyRequired(optional, required, false) {
	case requiredExactPresence:
		translated = append(translated, "required")
	case requiredOptionalPlain:
		sb = sb.Nillable().Optional()
	case requiredResidualZero:
		residual = append(residual, "required")
		sb = sb.Default(0)
	default:
		sb = sb.Default(0)
	}

	sf := finalizeSourceField(fd, scalarClass(optional), translated, residual, nil, entries)
	return sb.Annotations(sf), nil
}

// buildBoolField builds BoolKind. No Tier 1 range/format translation
// applies to bool (no such protovalidate constraint class); `required`
// still follows VAL-03's matrix — exact for a presence-tracking field
// (non-optional construction), residual for a plain one (bool has no
// typed .Validate()/.NotEmpty()-equivalent, D-12's established posture).
func buildBoolField(name string, fd protoreflect.FieldDescriptor, optional bool) (ent.Field, error) {
	required, celResidual, celEntries, err := resolvedFieldRules(fd)
	if err != nil {
		return nil, err
	}
	sb := field.Bool(name)
	var translated []string
	residual := append([]string{}, celResidual...)
	entries := append([]residualEntry{}, celEntries...)

	switch classifyRequired(optional, required, false) {
	case requiredExactPresence:
		translated = append(translated, "required")
	case requiredOptionalPlain:
		sb = sb.Nillable().Optional()
	case requiredResidualZero:
		residual = append(residual, "required")
		sb = sb.Default(false)
	default:
		sb = sb.Default(false)
	}

	sf := finalizeSourceField(fd, scalarClass(optional), translated, residual, nil, entries)
	return sb.Annotations(sf), nil
}

// buildStringField builds StringKind: the full VAL-01 translation
// (byte-semantic and code-point bounds, pattern, D-13's format-validator
// split) plus VAL-03's required handling. `required` is exact here, but
// NOT via a single uniform NotEmpty() call in both presence states —
// classifyRequired now resolves a presence-tracking (`optional`) field
// to requiredExactPresence (non-optional construction, no NotEmpty) and
// an implicit-presence (plain) field to requiredExactNotEmpty
// (NotEmpty()), matching protovalidate's own distinct semantics for the
// two cases (01-VERIFICATION.md gap 3 / 01-REVIEW.md CR-03).
func buildStringField(name string, fd protoreflect.FieldDescriptor, optional bool) (ent.Field, error) {
	required, celResidual, celEntries, err := resolvedFieldRules(fd)
	if err != nil {
		return nil, err
	}
	sb := field.String(name)
	var translated, divergent, residual []string
	var entries []residualEntry
	residual = append(residual, celResidual...)
	entries = append(entries, celEntries...)

	rules, rerr := protovalidate.ResolveFieldRules(fd)
	if rerr != nil {
		return nil, fmt.Errorf("resolving protovalidate field rules: %w", rerr)
	}
	if rules != nil {
		if sr := rules.GetString_(); sr != nil {
			sb, translated, divergent, err = applyStringConstraints(sb, sr)
			if err != nil {
				return nil, err
			}
			var fTranslated, fResidual string
			var fEntry *residualEntry
			sb, fTranslated, fResidual, fEntry, err = applyStringFormat(sb, fd, sr)
			if err != nil {
				return nil, err
			}
			if fTranslated != "" {
				translated = append(translated, fTranslated)
			}
			if fResidual != "" {
				residual = append(residual, fResidual)
				if fEntry != nil {
					entries = append(entries, *fEntry)
				}
			}
		}
	}

	switch classifyRequired(optional, required, true) {
	case requiredExactPresence:
		// Presence-tracking field + required: a presence assertion, not
		// a non-emptiness one. No NotEmpty(), no Nillable().Optional(),
		// no Default — mirrors buildInt32Field's requiredExactPresence
		// arm exactly. String/bytes stop being special-cased here.
		translated = append(translated, "required")
	case requiredExactNotEmpty:
		sb = sb.NotEmpty()
		translated = append(translated, "required")
	case requiredOptionalPlain:
		sb = sb.Nillable().Optional()
	default: // requiredNone (requiredResidualZero is unreachable for string/bytes: hasNotEmpty=true means required always hits requiredExactPresence or requiredExactNotEmpty above)
		sb = sb.Default("")
	}

	sf := finalizeSourceField(fd, scalarClass(optional), translated, residual, divergent, entries)
	return sb.Annotations(sf), nil
}

// buildBytesField builds BytesKind. bytesBuilder shares NotEmpty() with
// stringBuilder for the implicit-presence case, and shares the
// presence-only case with the non-string builders for the
// presence-tracking case (01-VERIFICATION.md gap 3 / 01-REVIEW.md
// CR-03) — so `required` is exact for both presence states, but not via
// one uniform mechanism. Byte-semantic min_len/max_len/pattern
// translation for bytes.* rules is out of this plan's scope (VAL-01
// names "string" specifically) and is left for a follow-up — a
// documented boundary, not a silent drop, since this plan's corpus
// carries no bytes.* length/pattern constraints to lose.
func buildBytesField(name string, fd protoreflect.FieldDescriptor, optional bool) (ent.Field, error) {
	required, celResidual, celEntries, err := resolvedFieldRules(fd)
	if err != nil {
		return nil, err
	}
	sb := field.Bytes(name)
	translated := []string{}
	residual := append([]string{}, celResidual...)
	entries := append([]residualEntry{}, celEntries...)

	rules, rerr := protovalidate.ResolveFieldRules(fd)
	if rerr != nil {
		return nil, fmt.Errorf("resolving protovalidate field rules: %w", rerr)
	}
	if rules != nil && rules.HasBytes() {
		r, e := recordBytesResidual(rules.GetBytes())
		residual = append(residual, r...)
		entries = append(entries, e...)
	}

	switch classifyRequired(optional, required, true) {
	case requiredExactPresence:
		// Mirrors buildStringField's requiredExactPresence arm: no
		// NotEmpty(), no Nillable().Optional(), no Default.
		translated = append(translated, "required")
	case requiredExactNotEmpty:
		sb = sb.NotEmpty()
		translated = append(translated, "required")
	case requiredOptionalPlain:
		sb = sb.Nillable().Optional()
	default:
		sb = sb.Default(nil)
	}

	sf := finalizeSourceField(fd, scalarClass(optional), translated, residual, nil, entries)
	return sb.Annotations(sf), nil
}

// mapEnum handles MIX-03: field.Enum(name).Values(...) built by
// iterating EnumDescriptor.Values() by index from 0 to Len()-1 and
// taking each .Name() — never via an intermediate map (D-24). The
// declared value names are carried verbatim, with no case
// transformation and no stripping of a leading ENUM_NAME_ prefix
// (Flagged Assumption MIX-03). enum.defined_only is satisfied by this
// construction itself and needs no separate residual record (D-14).
func mapEnum(fd protoreflect.FieldDescriptor) ent.Field {
	name := string(fd.Name())
	ed := fd.Enum()
	vals := ed.Values()
	names := make([]string, 0, vals.Len())
	for i := 0; i < vals.Len(); i++ {
		names = append(names, string(vals.Get(i).Name()))
	}
	sf := sourceFieldFor(fd, "enum")
	return field.Enum(name).Values(names...).Annotations(sf)
}

// mapScalarMap handles MIX-06's scalar-valued-map branch: one JSON
// field typed as a Go map of the mapped key and value Go types. Map
// constraints are recorded as residual, not enforced, in Phase 1
// (Flagged Assumption MIX-06) — Phase 3's concern, not this plan's.
func mapScalarMap(fd protoreflect.FieldDescriptor) ent.Field {
	name := string(fd.Name())
	sf := sourceFieldFor(fd, "scalarMap")
	typ := reflect.MakeMap(reflect.MapOf(goScalarType(fd.MapKey().Kind()), goScalarType(fd.MapValue().Kind()))).Interface()
	return field.JSON(name, typ).Annotations(sf)
}

// goScalarType returns the concrete Go type a protoreflect scalar Kind
// decodes to, for building a map[K]V typ argument to field.JSON. Only
// scalar kinds ever reach here: mapScalarMap is only called for map
// fields whose value kind is not MessageKind (classify already routed
// message-valued maps to classMessageMap, skipped before this is
// reached).
func goScalarType(k protoreflect.Kind) reflect.Type {
	switch k {
	case protoreflect.StringKind:
		return reflect.TypeOf("")
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return reflect.TypeOf(int32(0))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return reflect.TypeOf(int64(0))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return reflect.TypeOf(uint32(0))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return reflect.TypeOf(uint64(0))
	case protoreflect.BoolKind:
		return reflect.TypeOf(false)
	case protoreflect.DoubleKind:
		return reflect.TypeOf(float64(0))
	case protoreflect.FloatKind:
		return reflect.TypeOf(float32(0))
	case protoreflect.BytesKind:
		return reflect.TypeOf([]byte(nil))
	default:
		return reflect.TypeOf((*any)(nil)).Elem()
	}
}

// mapWellKnownType handles MIX-04: it returns handled=true if fd's
// message type is a recognized well-known type, whether that WKT maps
// to a field (Timestamp, Struct, Value) or is deliberately skipped
// (FieldMask, Duration — MIX2-04's Duration-to-nanoseconds mapping is
// v2). handled=false means fd is an ordinary (non-WKT) message field
// the caller must route through the message-field/AsJSON path instead.
//
// google.protobuf.Value maps to field.JSON typed json.RawMessage,
// chosen because it round-trips any JSON value losslessly where a
// concrete Go type would not (RESEARCH.md Open Question 2, closed by
// this plan). google.protobuf.Struct maps to field.JSON typed
// map[string]any, matching its own natural JSON shape.
func mapWellKnownType(fd protoreflect.FieldDescriptor) (f ent.Field, handled bool, err error) {
	if fd.Kind() != protoreflect.MessageKind || fd.IsList() {
		return nil, false, nil
	}
	name := string(fd.Name())
	sf := sourceFieldFor(fd, "wkt")
	switch string(fd.Message().FullName()) {
	case wktTimestamp:
		return field.Time(name).Annotations(sf), true, nil
	case wktStruct:
		return field.JSON(name, map[string]any{}).Annotations(sf), true, nil
	case wktValue:
		return field.JSON(name, json.RawMessage(nil)).Annotations(sf), true, nil
	case wktFieldMask, wktDuration:
		return nil, true, nil
	default:
		return nil, false, nil
	}
}

// mapAsJSON handles MIX-09's opt-in: a message field named in AsJSON(...)
// maps to a JSON field typed json.RawMessage, the same lossless
// representation used for google.protobuf.Value.
func mapAsJSON(fd protoreflect.FieldDescriptor) ent.Field {
	name := string(fd.Name())
	sf := sourceFieldFor(fd, "asJSON")
	return field.JSON(name, json.RawMessage(nil)).Annotations(sf)
}

// mapMessageField handles MIX-09's default path for any message-typed
// field that classify already confirmed is not a map: well-known types
// are mapped or skipped per mapWellKnownType; anything else is skipped
// by default unless the developer opted it in with AsJSON.
func mapMessageField(fd protoreflect.FieldDescriptor, o *options) (ent.Field, error) {
	if f, handled, err := mapWellKnownType(fd); handled {
		return f, err
	}
	if o.isAsJSON(string(fd.Name())) {
		return mapAsJSON(fd), nil
	}
	return nil, nil
}

// validateAsJSON checks every name passed to AsJSON(...) against md's
// real field set (MIX-09's failure surface): AsJSON("") and AsJSON
// naming a field that does not exist on the message each fail at schema
// load naming the message and the offending option; AsJSON naming a
// scalar (non-message) field also fails rather than silently succeeding.
// Failures are collected, not returned at the first offense (D-09), and
// names are visited in sorted order (o.asJSONNames(), D-24) so error
// output is diffable. Called from derive.go's validateOptionNames, so
// every option-name problem (Exclude/Override/AsJSON) lands in one
// failures slice.
func validateAsJSON(msgName string, md protoreflect.MessageDescriptor, o *options) []failure {
	var out []failure
	for _, name := range o.asJSONNames() {
		if name == "" {
			out = append(out, failure{
				message:     msgName,
				field:       "",
				fieldIndex:  fieldIndexUnnamed,
				rule:        "AsJSON",
				description: "AsJSON(\"\") field name must not be empty",
				remedy:      "name the message field you want serialized as JSON",
			})
			continue
		}
		fd := md.Fields().ByName(protoreflect.Name(name))
		if fd == nil {
			out = append(out, failure{
				message:     msgName,
				field:       name,
				fieldIndex:  fieldIndexUnnamed,
				rule:        "AsJSON",
				description: fmt.Sprintf("AsJSON(%q) names a field that does not exist on this message", name),
				remedy:      "check for a typo or a renamed contract field",
			})
			continue
		}
		if fd.Kind() != protoreflect.MessageKind {
			out = append(out, failure{
				message:     msgName,
				field:       name,
				fieldIndex:  int(fd.Index()),
				rule:        "AsJSON",
				description: fmt.Sprintf("AsJSON(%q) names a non-message field (kind %s)", name, fd.Kind()),
				remedy:      "AsJSON only applies to message-typed fields",
			})
		}
	}
	return out
}
