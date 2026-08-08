package mixinforproto

import "sort"

// Option configures a MixinForProto (or Validate[M]) derivation.
//
// Phase 1's walking skeleton declares no exported option constructors —
// Exclude and Override belong to Plan 03, AsJSON to Plan 02, and
// WithMessageRules is deliberately omitted from Phase 1 entirely (see
// SKELETON.md "Out of Scope"). This file exists now so derive.go and
// fieldmap.go can already call the lookup helpers below, and so Plans 02
// and 03 extend this file's options struct without reopening derive.go.
type Option func(*options)

// options is the unexported, accumulated configuration a set of Option
// values builds. Every field is a name set rather than a positional
// list, because Exclude/Override/AsJSON semantics are "does this proto
// field name appear here", never "in what order were options passed".
type options struct {
	excluded   map[string]bool
	overridden map[string]bool
	asJSON     map[string]bool
}

// applyOptions builds the effective options struct for one derive[M]
// call by folding every Option in order.
func applyOptions(opts []Option) *options {
	o := &options{
		excluded:   map[string]bool{},
		overridden: map[string]bool{},
		asJSON:     map[string]bool{},
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// isExcluded reports whether the named proto field was passed to
// Exclude(...).
func (o *options) isExcluded(name string) bool {
	return o.excluded[name]
}

// isOverridden reports whether the named proto field was passed to
// Override(...).
func (o *options) isOverridden(name string) bool {
	return o.overridden[name]
}

// isAsJSON reports whether the named proto field was passed to
// AsJSON(...).
func (o *options) isAsJSON(name string) bool {
	return o.asJSON[name]
}

// AsJSON opts a message-typed proto field into JSON-field derivation
// (MIX-09): by default, message-typed fields are skipped entirely.
// Naming a field that does not exist, an empty string, or a field that
// is not message-typed fails loudly at schema load — see
// fieldmap.go#validateAsJSON — never silently accepted or silently
// dropped.
func AsJSON(name string) Option {
	return func(o *options) {
		o.asJSON[name] = true
	}
}

// excludedNames returns the excluded field names in sorted order, so
// SourceMessage.Excluded is deterministic (D-24) regardless of the map's
// iteration order.
func (o *options) excludedNames() []string {
	return sortedKeys(o.excluded)
}

// overriddenNames returns the overridden field names in sorted order,
// for the same reason as excludedNames.
func (o *options) overriddenNames() []string {
	return sortedKeys(o.overridden)
}

// asJSONNames returns the AsJSON-opted-in field names in sorted order,
// for the same reason as excludedNames — this is what makes
// validateAsJSON's error output diffable (D-24).
func (o *options) asJSONNames() []string {
	return sortedKeys(o.asJSON)
}

// sortedKeys returns m's keys in sorted order — never range a map
// directly into ordered output (D-24).
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
