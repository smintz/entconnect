package schema

import (
	"entgo.io/ent"

	"github.com/smintz/entconnect/mixinforproto"
	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// DoubleComparators is Plan 03-05 Task 3's PIPE-06 differential-sweep fixture: a
// bare MixinForProto[*mixinforprototestv1.DoubleComparators]() with no options, so
// sweep_test.go can drive real Create()/Update() calls against it
// generically via reflection (ent.Mutation.SetField), without any
// DoubleComparators-specific Go code in this package. See internal/difftest/
// sweep_test.go's own doc comment for why this schema exists and why
// its Mixin() carries no options.
type DoubleComparators struct {
	ent.Schema
}

// Mixin returns the derived mixin — the only thing this schema declares.
func (DoubleComparators) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.DoubleComparators](),
	}
}
