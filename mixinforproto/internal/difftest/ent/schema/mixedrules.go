package schema

import (
	"entgo.io/ent"

	"github.com/smintz/entconnect/mixinforproto"
	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// MixedFieldRules declares MixinForProto against the generated
// MixedFieldRules message type (proto/mixinforprototest/v1/
// constraints.proto), Plan 03-03's fixture for settling
// 03-RESEARCH.md Pitfall 1 (the D-07/D-08 hybrid-evaluator convergence
// risk): one field ("both") carries BOTH a standard rule
// (string.min_len = 3) and a custom (buf.validate.field).cel rule
// ("starts with X"), one sibling carries only the standard rule, and one
// sibling carries only the custom rule. This is the real generated
// ent.Client counterpart to violation_test.go's direct hookState.evaluate
// tests: it proves the chosen resolution to Pitfall 1 (deduplicate by
// (RuleId, FieldPath) at violation.go's newValidationError, rather than
// splitting a mixed field's rule set across two evaluators) holds through
// ent's real withHooks pipeline, not just against a hand-built
// ent.Mutation double.
type MixedFieldRules struct {
	ent.Schema
}

// Mixin returns the derived mixin — the only thing this schema declares.
func (MixedFieldRules) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.MixedFieldRules](),
	}
}
