package mixinforproto

import "entgo.io/ent/schema"

// ContractVersion is the version of the mixinforproto annotation
// contract: the on-the-wire shape of SourceMessage and SourceField as
// they cross entc's schema-load JSON boundary. It is embedded on both
// structs so a downstream decoder (the Phase 2 entc extension, the
// Phase 5 drift check) can answer "can I decode this?" with a single
// integer comparison rather than parsing a version string.
//
// Bumped only on a breaking layout change. Between bumps the policy is
// additive-only: new fields may be added, existing fields are never
// renamed or repurposed, and decoding tolerates unknown keys. A consumer
// reading a ContractVersion higher than the one it was compiled against
// must fail with a named mismatch error, never silently decode a partial
// struct (D-03/ANNO-04).
const ContractVersion = 1

// Annotation keys, exported so downstream decoders reference the
// constant rather than re-typing the literal — mirrors entproto's
// MessageAnnotation = "ProtoMessage" precedent (D-04).
const (
	// MixinForProtoMessage is the schema.Annotation key under which
	// SourceMessage is stored on gen.Type.Annotations.
	MixinForProtoMessage = "MixinForProtoMessage"
	// MixinForProtoField is the schema.Annotation key under which
	// SourceField is stored on gen.Field.Annotations.
	MixinForProtoField = "MixinForProtoField"
)

// FieldRef is one entry in SourceMessage's complete descriptor field
// inventory: every proto field's name and number as they appear in the
// message descriptor at derivation time, regardless of whether that
// field was included, excluded, or overridden.
//
// This inventory is load-bearing (D-02): without it, a later drift check
// cannot distinguish "deliberately excluded" from "silently forgotten"
// — both look identical as absence from gen.Graph.
type FieldRef struct {
	Name   string `json:"name"`
	Number int32  `json:"number"`
}

// SourceMessage is the schema-level provenance annotation written by a
// MixinForProto mixin's Annotations() method. It is the only channel
// across entc's schema-load JSON boundary for message-level provenance
// (ANNO-01): load.Schema.Field.Validators is an int count, not closures,
// so nothing but an annotation survives the subprocess round trip.
type SourceMessage struct {
	ContractVersion int        `json:"contractVersion"`
	Message         string     `json:"message"`
	Fields          []FieldRef `json:"fields"`
	Excluded        []string   `json:"excluded"`
	Overridden      []string   `json:"overridden"`
}

// Name implements entgo.io/ent/schema.Annotation.
func (SourceMessage) Name() string {
	return MixinForProtoMessage
}

// SourceField is the field-level provenance annotation attached to each
// derived ent.Field via the field builder's own Annotations() method
// (ANNO-02). TranslatedIDs and ResidualIDs are populated by Plan 04's
// Tier 1 constraint translation; Plan 01 (the walking skeleton) leaves
// them present but empty.
//
// The proto field name lives in the Go field FieldName, not Name: a
// struct cannot declare both a data field and a method named Name in
// Go, and Name() string is fixed by schema.Annotation. The wire/JSON key
// stays "name" (json:"name") so the annotation contract's on-the-wire
// shape matches the checkpoint-resolved layout exactly — only the Go
// accessor identifier differs from the decision text.
type SourceField struct {
	ContractVersion int    `json:"contractVersion"`
	FieldName       string `json:"name"`
	Number          int32  `json:"number"`
	// Kind is the derivation kind (scalar / enum / WKT / map-as-JSON /
	// message-as-JSON / override). Plan 01 only ever writes the raw
	// protoreflect.Kind string for the one scalar case it implements.
	Kind string `json:"kind"`
	// TranslatedIDs are the protovalidate constraint IDs this field's
	// derivation translated into native ent builder calls (Tier 1).
	TranslatedIDs []string `json:"translatedIDs"`
	// ResidualIDs are the protovalidate constraint IDs Tier 1 could not
	// translate exactly; they are recorded, not enforced, until Phase 3.
	ResidualIDs []string `json:"residualIDs"`
	// ResidualFingerprint is a stable fingerprint of the residual
	// constraints' CEL expression strings, the handoff Phase 3 uses to
	// detect that the recorded residuals still match the contract.
	ResidualFingerprint string `json:"residualFingerprint"`
}

// Name implements entgo.io/ent/schema.Annotation.
func (SourceField) Name() string {
	return MixinForProtoField
}

var (
	_ schema.Annotation = SourceMessage{}
	_ schema.Annotation = SourceField{}
)
