// Package schema is a real ent schema package for the driverless
// differential harness (D-13/PIPE-06): a real generated ent.Client built
// against it, driven by fakedriver.go's hand-written dialect.Driver, is
// what proves ent's own withHooks pipeline actually invokes
// mixinforproto's new Hooks() entry (hooks.go) on a real Save(ctx) call —
// a property no unit test of hooks.go in isolation can show, exactly the
// way mixinforproto/internal/boundarytest's schema-load-only harness
// cannot show it either (boundarytest never builds a client at all).
package schema

import (
	"entgo.io/ent"

	"github.com/smintz/entconnect/mixinforproto"
	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// ResidualCel declares MixinForProto against the generated ResidualCel
// message type (proto/mixinforprototest/v1/constraints.proto): one
// string field carrying a single (buf.validate.field).cel rule
// ("constraints.residual_cel.starts_with_x") that Tier 1 could never
// translate and that, until this plan, was recorded on
// SourceField.ResidualIDs and enforced by nothing. This is the thinnest
// possible schema that exercises hooks.go's new residual-CEL evaluation
// path end to end.
type ResidualCel struct {
	ent.Schema
}

// Mixin returns the derived mixin — the only thing this schema declares.
func (ResidualCel) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.ResidualCel](),
	}
}
