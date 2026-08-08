// Package fieldproj builds the golden-comparable projection of a derived
// ent.Field used by mixinforproto's Phase 1 conformance corpus tests.
//
// field.Descriptor() (entgo.io/ent/schema/field) carries a Validators
// []any slice of live closures and a non-marshalable Err field —
// json.Marshal on the raw *field.Descriptor errors the moment any Tier 1
// rule attaches a validator (01-RESEARCH.md Correction 2, applied here
// as R4 per 01-02-PLAN.md). fieldproj.Project builds a hand-written,
// entirely JSON-safe projection instead, and never marshals the raw
// descriptor.
//
// fieldproj deliberately does not import mixinforproto: it is imported
// by mixinforproto's own (in-package) test files, so importing
// mixinforproto here would be an import cycle. SourceFieldProjection
// instead mirrors mixinforproto.SourceField's JSON wire shape and is
// populated via an encoding/json round-trip off the real annotation
// value, not a direct type reference.
package fieldproj

import (
	"encoding/json"
	"fmt"
	"reflect"

	"entgo.io/ent"
)

// EnumValue mirrors entgo.io/ent/schema/field.Descriptor's Enums entry
// shape (an anonymous struct{ N, V string } with no json tags) as a
// named, JSON-tagged struct, so the golden fixture is stable and
// readable rather than depending on the anonymous struct's default
// {"N":...,"V":...} marshaling.
type EnumValue struct {
	N string `json:"n"`
	V string `json:"v"`
}

// SourceFieldProjection mirrors mixinforproto.SourceField's JSON wire
// shape. See the package doc comment for why this is a hand-mirrored
// struct rather than a direct import.
type SourceFieldProjection struct {
	ContractVersion     int      `json:"contractVersion"`
	FieldName           string   `json:"name"`
	Number              int32    `json:"number"`
	Kind                string   `json:"kind"`
	TranslatedIDs       []string `json:"translatedIDs"`
	ResidualIDs         []string `json:"residualIDs"`
	ResidualFingerprint string   `json:"residualFingerprint"`
}

// Field is the golden-comparable projection of one derived ent.Field's
// Descriptor(). Every member is a JSON-safe scalar or slice with an
// explicit json tag; marshaling a Field never errors even when the
// source descriptor carries validator closures.
type Field struct {
	Name           string                 `json:"name"`
	TypeKind       string                 `json:"typeKind"`
	Nillable       bool                   `json:"nillable"`
	Optional       bool                   `json:"optional"`
	Immutable      bool                   `json:"immutable"`
	Default        string                 `json:"default"`
	ValidatorCount int                    `json:"validatorCount"`
	Enums          []EnumValue            `json:"enums,omitempty"`
	StorageKey     string                 `json:"storageKey"`
	Comment        string                 `json:"comment"`
	SourceField    *SourceFieldProjection `json:"sourceField,omitempty"`
}

// Project builds the golden-comparable projection of one derived
// ent.Field. It never marshals the raw *field.Descriptor directly (see
// package doc comment).
func Project(f ent.Field) Field {
	d := f.Descriptor()

	enums := make([]EnumValue, 0, len(d.Enums))
	for _, e := range d.Enums {
		enums = append(enums, EnumValue{N: e.N, V: e.V})
	}

	p := Field{
		Name:           d.Name,
		TypeKind:       d.Info.Type.String(),
		Nillable:       d.Nillable,
		Optional:       d.Optional,
		Immutable:      d.Immutable,
		Default:        stableDefault(d.Default),
		ValidatorCount: len(d.Validators),
		Enums:          enums,
		StorageKey:     d.StorageKey,
		Comment:        d.Comment,
	}

	// Field-level provenance annotations (SourceField) are the only
	// mixinforproto-specific annotation attached per field (D-02); decode
	// the first one found via a JSON round-trip rather than a direct
	// type reference (see package doc comment).
	for _, a := range d.Annotations {
		raw, err := json.Marshal(a)
		if err != nil {
			continue
		}
		var sf SourceFieldProjection
		if err := json.Unmarshal(raw, &sf); err != nil {
			continue
		}
		p.SourceField = &sf
		break
	}

	return p
}

// ProjectAll projects every field in fs, preserving input order. It must
// never sort — sorting here would hide exactly the determinism
// regression the golden tests exist to catch (D-24/MIX-13).
func ProjectAll(fs []ent.Field) []Field {
	out := make([]Field, 0, len(fs))
	for _, f := range fs {
		out = append(out, Project(f))
	}
	return out
}

// stableDefault renders a field's Default value as a stable string. A
// nil Default (the common case for a field with no default — e.g. an
// `optional` scalar, an enum, or a message-derived JSON field) always
// renders as the empty string, never the literal Go "<nil>" spelling
// that can vary by build. A func-valued Default (DefaultFunc, not used
// by this plan's derivation but defensively handled here) renders as a
// fixed placeholder rather than an address that would vary across runs
// and break the byte-identical golden guarantee (D-24).
//
// Non-nil values render via the "%#v" Go-syntax verb, not fmt.Sprint:
// a plain (non-optional) proto3 string field's Default is the Go zero
// value "", which fmt.Sprint renders as the empty string — byte-for-byte
// indistinguishable from "no default was set" (the nil case above).
// "%#v" renders it as the 2-character `""`, so "a default is present"
// is always detectable from the rendered string alone, which is exactly
// the distinction MIX-05's plain-vs-optional golden assertions depend
// on (a plain scalar must show a non-empty Default; an optional scalar
// must show none at all).
func stableDefault(v any) string {
	if v == nil {
		return ""
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Func {
		return "<func>"
	}
	return fmt.Sprintf("%#v", v)
}
