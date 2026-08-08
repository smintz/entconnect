package mixinforproto

// reservedStatic is the hand-copied, case-sensitive union of
// entc/gen/type.go's globalIdent (26 entries) and privateField (16
// entries) — 41 unique identifiers after deduplicating "config", the
// only identifier present in both lists. Copied, not imported:
// importing entc/gen would pull codegen machinery into the runtime
// mixin and break the module-isolation constraint that is MIX-14 (see
// 01-RESEARCH.md's "Common Pitfalls -> Pitfall 2" and this plan's
// Reconciliation R3).
//
// [VERIFIED: github.com/ent/ent, entc/gen/type.go, fetched 2026-08-08]
var reservedStatic = map[string]bool{
	// globalIdent (26)
	"AggregateFunc": true,
	"As":            true,
	"Asc":           true,
	"Client":        true,
	"config":        true,
	"Count":         true,
	"Debug":         true,
	"Desc":          true,
	"Driver":        true,
	"Hook":          true,
	"Interceptor":   true,
	"Log":           true,
	"MutateFunc":    true,
	"Mutation":      true,
	"Mutator":       true,
	"Op":            true,
	"Option":        true,
	"OrderFunc":     true,
	"Max":           true,
	"Mean":          true,
	"Min":           true,
	"Schema":        true,
	"Sum":           true,
	"Policy":        true,
	"Query":         true,
	"Value":         true,
	// privateField (16; "config" above already covers the one overlap)
	"ctx":        true,
	"done":       true,
	"hooks":      true,
	"inters":     true,
	"limit":      true,
	"mutation":   true,
	"offset":     true,
	"oldValue":   true,
	"order":      true,
	"op":         true,
	"path":       true,
	"predicates": true,
	"typ":        true,
	"unique":     true,
	"driver":     true,
}

// reservedStructural is the short, explicitly incomplete supplement for
// identifiers ent generates *per type* rather than from any static
// list: every entity has an id, and ent/ent#280 confirms a per-type
// Label constant collides the same way (a Go compiler "redeclared in
// this block" error, not a schema-load panic from mixinforproto — that
// issue predates this mixin entirely).
//
// This category cannot be enumerated — the identifiers are derived from
// each entity's own name/fields — so it is deliberately NOT extended to
// type/edge/where (see this plan's Reconciliation R3): those are named
// in CONTEXT.md D-10 but were not verified against ent's source in
// 01-RESEARCH.md, and adding them speculatively would reject legitimate
// contract field names and force adopters into an unnecessary Override.
// Collisions this list does not cover — a hand-declared schema field,
// another mixin's field, or a structural collision outside {id, label}
// — surface as the Go compiler's own redeclaration error instead,
// exactly as D-10 already documents as an accepted limitation.
var reservedStructural = map[string]bool{
	"id":    true,
	"label": true,
}

// isReserved reports whether name collides with either reserved
// catalog, matched case-sensitively: "Config" does not collide with the
// static entry "config" unless that exact spelling is present.
func isReserved(name string) bool {
	return reservedStatic[name] || reservedStructural[name]
}
