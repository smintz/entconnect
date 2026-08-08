// Package mixinforproto derives ent fields — including translated
// validation — from a generated protobuf message type at ent schema-load
// time, so the protobuf contract is the single, one-time definition of a
// schema's API-visible fields. It is a runtime mixin: plain Go, no entc
// extension, no code generation, no committed descriptor file.
//
// Before writing an Update handler against a mixinforproto-derived field,
// read the README's "Proto3 presence and the zero-collapse" section
// (https://github.com/smintz/entconnect/blob/main/mixinforproto/README.md#proto3-presence-and-the-zero-collapse-read-this-before-you-write-an-update-handler)
// — a non-optional proto3 scalar collapses "unset" and "zero" to the same
// value, and this package cannot fix that for you at this layer.
//
// If your contract has non-ASCII string data, read the README's "String
// length: Unicode code points vs. bytes" section
// (https://github.com/smintz/entconnect/blob/main/mixinforproto/README.md#string-length-unicode-code-points-vs-bytes-read-this-if-your-contract-has-non-ascii-string-data)
// — string.min_len/max_len/len count Unicode code points; the derived
// ent field's MinLen/MaxLen count bytes. This package maps them directly
// anyway (a deliberate, recorded decision, not an oversight); the
// divergence is machine-visible on SourceField.LengthUnitDivergentIDs.
//
// A mixin's Hooks, Interceptors, and Policy all run before the ones a
// schema author declares directly on the schema — see the README's
// "Mixin hook and policy ordering" section for the full note, including
// what this release does and does not enforce.
package mixinforproto
