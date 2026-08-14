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
