package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	"github.com/smintz/entconnect/mixinforproto"
	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// OverriddenMixedFieldRules declares MixinForProto against the generated
// MixedFieldRules message type (proto/mixinforprototest/v1/
// constraints.proto — the same fixture 03-VERIFICATION.md's own probe
// used to falsify CR-03), with Override("both", field.String("both"))
// applied. This is the real generated ent.Client counterpart to
// hooks_test.go's direct hookState.evaluate Override tests: it proves
// through ent's real withHooks pipeline (D-13) — not just a hand-built
// ent.Mutation double — that a real Create setting "both" to a
// rule-violating value succeeds, while "cel_only" still rejects with its
// own RuleId. This is what closes the gap-closure contract's "no test
// calls Override/Exclude together with a hook-bearing schema and asserts
// zero violations" finding.
type OverriddenMixedFieldRules struct {
	ent.Schema
}

// Mixin returns the derived mixin — the only thing this schema declares.
func (OverriddenMixedFieldRules) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.MixedFieldRules](
			mixinforproto.Override("both", field.String("both")),
		),
	}
}
