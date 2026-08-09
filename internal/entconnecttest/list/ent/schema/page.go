// Package schema is the List slice fixture's real ent schema package —
// the entc extension's write-side proof for OpList, mirroring the read
// fixture's own schema package shape (internal/entconnecttest/read/ent/schema).
package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"

	entconnect "github.com/smintz/entconnect/entc"
	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"
	"github.com/smintz/entconnect/mixinforproto"
)

// Page declares MixinForProto against the generated Page message type
// and binds PageListService's ListPages RPC via entconnect.ListRPC — a
// compile-time-checked reference to entconnecttestv1connect's generated
// procedure constant. The orderBy argument names created_at, a
// deliberately non-unique field (see list.proto's own comment): the
// entity's primary key is appended last to the ordering tuple by the
// OpList generator itself (CRUD-03's tie-breaking guarantee), so the
// fixture's own tests can drive a page boundary directly between two
// tied created_at values.
type Page struct {
	ent.Schema
}

// Mixin derives title/created_at from the Page message. "id" is excluded
// for the same reason the read fixture's Order.id is excluded
// (mixinforproto/reserved.go's reservedStructural) — this entity uses
// ent's own default auto-incrementing int ID, and the generated ListPages
// handler converts between the two at the boundary.
func (Page) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.Page](mixinforproto.Exclude("id")),
	}
}

// Annotations binds this schema's List RPC (D-01), ordering by
// created_at with the primary key appended last by the generator.
func (Page) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.ListRPC(entconnecttestv1connect.PageListServiceListPagesProcedure, "created_at"),
	}
}
