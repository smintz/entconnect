// Package schema is a real ent schema package, loaded by a real entc
// schema-load subprocess in boundary_test.go. It is the walking
// skeleton's proof surface: the one place this plan runs the actual
// gorun() subprocess boundary the mixin's annotation-only design exists
// to survive.
package schema

import (
	"entgo.io/ent"

	"github.com/smintz/entconnect/mixinforproto"
	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// Tracer declares MixinForProto against the generated Tracer message
// type, matching the literal API shape in mixinforproto.md §1: no
// committed descriptor file, no string message name anywhere in the
// call (MIX-01).
type Tracer struct {
	ent.Schema
}

// Mixin returns the derived mixin. This is the only place this schema
// declares anything at all — Fields/Edges/Indexes/Hooks/Policy all stay
// at ent.Schema's defaults, and MixinForProto supplies everything.
func (Tracer) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.Tracer](),
	}
}
