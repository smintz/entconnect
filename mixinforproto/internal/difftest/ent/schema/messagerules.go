package schema

import (
	"entgo.io/ent"

	"github.com/smintz/entconnect/mixinforproto"
	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// MessageRules declares MixinForProto against the generated MessageRuleOk
// message type (proto/mixinforprototest/v1/messagerules.proto) with
// WithMessageRules(mixinforproto.OnCreate) set — Plan 03-05's fixture
// proving VAL-08/D-10's opt-in through a real ent.Client's withHooks
// pipeline (PIPE-06), not just against a hand-built ent.Mutation double:
// a real Create().Save(ctx) violating "this.lo <= this.hi" is rejected
// carrying the message rule's own RuleId (Test 2), and a real
// Update().Save(ctx) is NOT subject to message-rule enforcement under
// the same option (Test 3, Create-only opt-in).
type MessageRules struct {
	ent.Schema
}

// Mixin returns the derived mixin — the only thing this schema declares.
func (MessageRules) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.MessageRuleOk](
			mixinforproto.WithMessageRules(mixinforproto.OnCreate),
		),
	}
}

// MessageRuleCelExpression declares MixinForProto against the generated
// MessageRuleCelExpressionOk message type (proto/mixinforprototest/v1/
// messagerules.proto) with WithMessageRules(mixinforproto.OnCreate) set —
// 03-08-PLAN.md Task 2's real-ent.Client fixture for CR-01's
// cel_expression carrier gap closure: a real Create violating
// "this.lo <= this.hi" (declared via the simplified cel_expression
// carrier, not `cel`) is rejected through ent's actual withHooks
// pipeline (PIPE-06), not just against a hand-built ent.Mutation double.
type MessageRuleCelExpression struct {
	ent.Schema
}

// Mixin returns the derived mixin — the only thing this schema declares.
func (MessageRuleCelExpression) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.MessageRuleCelExpressionOk](
			mixinforproto.WithMessageRules(mixinforproto.OnCreate),
		),
	}
}

// MessageRuleOneof declares MixinForProto against the generated
// MessageRuleOneofOk message type (proto/mixinforprototest/v1/
// messagerules.proto) with WithMessageRules(mixinforproto.OnCreate) set —
// 03-08-PLAN.md Task 2's real-ent.Client fixture for CR-01's oneof
// carrier gap closure: a real Create violating the oneof rule (declared
// via the `oneof` carrier, no CEL involved at all) is rejected through
// ent's actual withHooks pipeline (PIPE-06).
type MessageRuleOneof struct {
	ent.Schema
}

// Mixin returns the derived mixin — the only thing this schema declares.
func (MessageRuleOneof) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.MessageRuleOneofOk](
			mixinforproto.WithMessageRules(mixinforproto.OnCreate),
		),
	}
}
