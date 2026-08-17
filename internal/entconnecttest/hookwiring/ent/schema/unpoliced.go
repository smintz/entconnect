// Package schema — see recorder.go's package doc.
package schema

import (
	"context"

	"entgo.io/ent"

	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/mixinforproto"
)

// Unpoliced is byte-identical to Policed (policed.go) apart from declaring
// NO Policy() at all — see Policed's package doc for why both fixtures
// exist. With no Policy() anywhere on this schema, ent's generated wiring
// never installs the combined privacy Hooks[0] wrapper, so the mixin's
// validation hook simply IS Hooks[0] here — ordering_test.go proves that
// too, by relative sequence, never by hard-coding that positional fact as
// an index.
type Unpoliced struct {
	ent.Schema
}

// Mixin — identical derivation to Policed.Mixin, same message type, same
// exclusion.
func (Unpoliced) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.HookWiring](mixinforproto.Exclude("id")),
	}
}

// Hooks — identical to Policed.Hooks: records a "schema-hook" marker, run
// only when the mixin's own validation hook (which precedes it, mixin
// hooks always ordering ahead of schema-declared hooks) let the mutation
// through.
func (Unpoliced) Hooks() []ent.Hook {
	return []ent.Hook{
		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				if r, ok := RecorderFromContext(ctx); ok {
					r.Append("schema-hook")
				}
				return next.Mutate(ctx, m)
			})
		},
	}
}
