package mixinforproto

import (
	"sort"

	"entgo.io/ent"
)

// Option configures a MixinForProto (or Validate[M]) derivation.
//
// WithMessageRules IS now declared (messagerules.go, Plan 03-05): message-
// level (cross-field) rules stay boundary-only by default, and
// WithMessageRules(OnCreate) opts a schema into storage-layer enforcement
// of them on Create only. What remains deliberately absent is
// OnUpdateWithFetch — a hypothetical Update-side trigger that would fetch
// the pre-mutation row to reconstruct a complete entity before evaluating
// a message-level rule — named in mixinforproto.md §4.3 as
// deliberately-excluded-from-v1 (hidden query cost, unclear semantics
// under concurrent writes). Declaring it today would be exactly the kind
// of no-op-that-lies symbol Phase 1's own precedent rejects (see this
// comment's prior wording, retained in spirit): a real symbol only ships
// once its behavior is real.
type Option func(*options)

// options is the unexported, accumulated configuration a set of Option
// values builds. excluded/asJSON are name sets; overridden maps each
// named field to its supplied replacement — the map key's presence
// (not the value) records that Override was called at all, so a nil
// replacement is distinguishable from "never overridden" rather than
// silently treated as "not overridden" (MIX-08 empty edge).
//
// messageRulesSet/messageRulesTrigger record WithMessageRules the same
// presence-vs-value way overridden does: messageRulesSet is true the
// moment WithMessageRules(trigger) is called AT ALL, regardless of
// whether trigger is a value this package declares, and
// messageRulesTrigger holds exactly the value the caller passed — never
// silently collapsed to OnCreate. WR-04 gap closure (03-08-PLAN.md): the
// PRIOR wording here (a bare bool, with a comment claiming "the trigger
// argument's own value is never stored, since MessageRuleTrigger
// declares exactly one legal value today") was the defect, not a
// simplification of it — an undeclared trigger silently behaved as
// OnCreate instead of failing schema load. The trigger's own validity is
// checked in messagerules.go's checkMessageRuleReferences, the moment
// messageRulesEnabled() reports true — see that function's own doc
// comment.
type options struct {
	excluded            map[string]bool
	overridden          map[string]ent.Field
	asJSON              map[string]bool
	messageRulesSet     bool
	messageRulesTrigger MessageRuleTrigger
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

// messageRulesEnabled reports whether WithMessageRules was passed to
// this derivation's options AT ALL, with any trigger value — valid or
// not. It deliberately does NOT report whether the trigger was OnCreate
// specifically: an undeclared trigger must still reach
// checkMessageRuleReferences (messagerules.go) to fail schema load
// loudly (WR-04), rather than messageRulesEnabled() silently reporting
// "not opted in" and skipping that check entirely — that would be the
// exact bug this gap closure fixes, just moved one function over.
func (o *options) messageRulesEnabled() bool {
	return o.messageRulesSet
}

// messageRulesTriggerValue returns the exact MessageRuleTrigger value
// passed to WithMessageRules, or OnCreate's zero value if
// WithMessageRules was never called — messageRulesEnabled() is the
// presence check (mirroring isOverridden/overriddenField's own
// presence-vs-value split above); a caller that cares whether
// WithMessageRules was called at all must check messageRulesEnabled()
// first, exactly as buildHookState already does.
func (o *options) messageRulesTriggerValue() MessageRuleTrigger {
	return o.messageRulesTrigger
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

// WithMessageRules opts a schema into storage-layer enforcement of its
// message's message-level (cross-field) protovalidate rules — boundary-
// only by default (VAL-08). trigger's only declared value is OnCreate
// (messagerules.go): the mixin's hook enforces message-level rules on
// Create only. Passing WithMessageRules(OnCreate) to a message that
// declares no message-level rules at all is a legal no-op — schema load
// succeeds and no evaluation is added.
//
// trigger's value IS stored and IS checked (WR-04 gap closure,
// 03-08-PLAN.md): passing any value other than OnCreate — an undeclared
// MessageRuleTrigger — fails schema load naming WithMessageRules and the
// offending numeric value (messagerules.go's checkMessageRuleReferences).
// Before this gap closure, trigger's value was discarded entirely and
// every call silently behaved as OnCreate; that let a hypothetical future
// caller believe a message's rules were enforced under some other trigger
// (e.g. an Update-side one) when nothing was actually enforced there at
// all — a declared-but-inert option is worse than a loud failure.
//
// Opting in is a deliberate act, and it carries a deliberate schema-load
// cost: every message-level rule's this.<field> selects (or, for a
// `oneof` rule, its literal named fields) are statically enumerated at
// schema load, and a reference to a field this package cannot
// reconstruct — excluded via Exclude, replaced via Override, or
// underivable — fails schema load naming the message, the rule id, and
// the field (D-10; see messagerules.go's checkMessageRuleReferences).
// This is deliberately stricter than the plain field-level case: a
// message-level rule computed against a field's phantom proto3 zero
// would return a real pass/fail verdict from data that was never real,
// which is worse than refusing to load at all.
func WithMessageRules(trigger MessageRuleTrigger) Option {
	return func(o *options) {
		o.messageRulesSet = true
		o.messageRulesTrigger = trigger
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
