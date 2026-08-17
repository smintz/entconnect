package schema

import (
	"entgo.io/ent"

	"github.com/smintz/entconnect/mixinforproto"
	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// StringByteBounds is Plan 03-05 Task 3's PIPE-06 differential-sweep fixture: a
// bare MixinForProto[*mixinforprototestv1.StringByteBounds]() with no options, so
// sweep_test.go can drive real Create()/Update() calls against it
// generically via reflection (ent.Mutation.SetField), without any
// StringByteBounds-specific Go code in this package. See internal/difftest/
// sweep_test.go's own doc comment for why this schema exists and why
// its Mixin() carries no options.
type StringByteBounds struct {
	ent.Schema
}

// Mixin returns the derived mixin — the only thing this schema declares.
func (StringByteBounds) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.StringByteBounds](),
	}
}
