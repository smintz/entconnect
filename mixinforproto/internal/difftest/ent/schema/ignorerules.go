package schema

import (
	"entgo.io/ent"

	"github.com/smintz/entconnect/mixinforproto"
	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// IgnoreAlwaysWithCel declares MixinForProto against the generated
// IgnoreAlwaysWithCel message type (proto/mixinforprototest/v1/
// ignore.proto), Plan 03-06 Task 1's fixture for CR-02's gap closure:
// "always_ignored" carries BOTH ignore = IGNORE_ALWAYS and a custom cel
// rule a real value can fail; "enforced" carries the identical rule
// shape with no ignore option at all. This is the real generated
// ent.Client counterpart to hooks_test.go's direct hookState.evaluate
// tests: it proves the IGNORE_ALWAYS branch holds through ent's real
// withHooks pipeline (D-13), not just against a hand-built ent.Mutation
// double.
type IgnoreAlwaysWithCel struct {
	ent.Schema
}

// Mixin returns the derived mixin — the only thing this schema declares.
func (IgnoreAlwaysWithCel) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.IgnoreAlwaysWithCel](),
	}
}

// IgnoreIfZeroWithCel declares MixinForProto against the generated
// IgnoreIfZeroWithCel message type (proto/mixinforprototest/v1/
// ignore.proto), Plan 03-06 Task 2's fixture: "zeroable" (string) and
// "zeroable_num" (int32) both carry ignore = IGNORE_IF_ZERO_VALUE plus a
// cel rule a real value can fail — the real generated ent.Client
// counterpart to hooks_test.go's direct hookState.evaluate zero-value
// tests, proving the mutation-time gate through ent's real withHooks
// pipeline (D-13).
type IgnoreIfZeroWithCel struct {
	ent.Schema
}

// Mixin returns the derived mixin — the only thing this schema declares.
func (IgnoreIfZeroWithCel) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.IgnoreIfZeroWithCel](),
	}
}
