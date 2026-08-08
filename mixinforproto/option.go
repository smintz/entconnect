package mixinforproto

import (
	"sort"

	"entgo.io/ent"
)

// Option configures a MixinForProto (or Validate[M]) derivation.
//
// WithMessageRules is deliberately not declared here: its behavior is
// Tier 3 (Phase 3) and a no-op symbol today would be an API that lies
// (see 01-04-PLAN.md's action text for Task 1).
type Option func(*options)

// options is the unexported, accumulated configuration a set of Option
// values builds. excluded/asJSON are name sets; overridden maps each
// named field to its supplied replacement — the map key's presence
// (not the value) records that Override was called at all, so a nil
// replacement is distinguishable from "never overridden" rather than
// silently treated as "not overridden" (MIX-08 empty edge).
type options struct {
	excluded   map[string]bool
	overridden map[string]ent.Field
	asJSON     map[string]bool
}

// applyOptions builds the effective options struct for one derive[M]
// call by folding every Option in order.
func applyOptions(opts []Option) *options {
	o := &options{
		excluded:   map[string]bool{},
		overridden: map[string]ent.Field{},
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

// isOverridden reports whether Override(name, ...) was called for name
// at all, regardless of whether the supplied replacement was nil — a
// nil replacement is a collected failure (see validateOptionNames), not
// silently "not overridden". This is also what the unresolved-oneof
// gate (Task 2) treats as "this member is resolved".
func (o *options) isOverridden(name string) bool {
	_, ok := o.overridden[name]
	return ok
}

// overriddenField returns the replacement field supplied to
// Override(name, f) and whether Override was called for name at all.
// The returned field may itself be nil; the caller decides what that
// means (validateOptionNames treats it as a failure; derive's per-field
// walk treats it as "install nothing", since the failure already covers
// the diagnostic).
func (o *options) overriddenField(name string) (ent.Field, bool) {
	f, ok := o.overridden[name]
	return f, ok
}

// isAsJSON reports whether the named proto field was passed to
// AsJSON(...).
func (o *options) isAsJSON(name string) bool {
	return o.asJSON[name]
}

// Exclude marks proto field names as not materialized into derived ent
// fields (MIX-07). Exclude() with no arguments is a legal no-op — the
// full field set derives normally. Naming a field that does not exist
// on the message (including the empty string) fails at schema load,
// naming the offending option and field rather than being silently
// ignored — see validateOptionNames in derive.go (D-09's collected-
// failures rule). Repeated Exclude calls, and multiple names in one
// call, are all additive.
func Exclude(names ...string) Option {
	return func(o *options) {
		for _, n := range names {
			o.excluded[n] = true
		}
	}
}

// Override replaces a derived field wholesale with a hand-declared one
// (MIX-08): f is installed verbatim, with no SourceField constraint
// provenance attached, because an override suppresses validation relay
// for that field entirely — the developer owns it completely, with no
// silent merging (D-05, mixinforproto.md §2). Naming a field that does
// not exist, or supplying a nil f, each fail at schema load naming the
// field, rather than installing a nil ent.Field that would panic far
// from its cause at codegen time. A name passed to both Exclude and
// Override is itself a reported conflict, not a silent precedence.
func Override(name string, f ent.Field) Option {
	return func(o *options) {
		o.overridden[name] = f
	}
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
	keys := make([]string, 0, len(o.overridden))
	for k := range o.overridden {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
