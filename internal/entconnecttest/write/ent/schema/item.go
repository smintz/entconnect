// Package schema is the write-slice fixture's real ent schema package —
// mirrors internal/entconnecttest/read/ent/schema's shape exactly, one
// entity proving the write path (Create in this plan's Task 1, Delete
// added by Task 2 on the same schema, exercising Bindings' merge path).
package schema

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/privacy"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/index"

	entconnect "github.com/smintz/entconnect/entc"
	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"
	"github.com/smintz/entconnect/mixinforproto"
	"github.com/smintz/entconnect/runtime/viewer"
)

// DeniedSubject is a sentinel viewer.Viewer.Subject() value the fixture's
// privacy policy below denies — create_test.go/delete_test.go use it to
// prove a real ent privacy denial on a mutation surfaces as Connect's
// PermissionDenied (INT-03/T-02-09).
const DeniedSubject = "denied-subject"

// Item declares MixinForProto against the generated Item message type
// (D-01: no committed descriptor file, no string message name anywhere
// in the call) and binds ItemCreateService's CreateItem RPC and
// ItemDeleteService's DeleteItem RPC via entconnect.CreateRPC/DeleteRPC —
// compile-time-checked references to entconnecttestv1connect's generated
// procedure constants.
type Item struct {
	ent.Schema
}

// Mixin derives sku/name/quantity from the Item message. "id" is
// excluded: it collides with ent's own reserved, per-type "id"
// identifier (mixinforproto/reserved.go's reservedStructural), so this
// entity uses ent's own default auto-incrementing int ID instead — the
// generated Create/Delete handlers convert between the two at the
// boundary.
func (Item) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.Item](mixinforproto.Exclude("id")),
	}
}

// Indexes declares a unique constraint on sku — the real constraint the
// double-create idempotency case (CRUD-02) needs to violate.
func (Item) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sku").Unique(),
	}
}

// Annotations binds this schema's Create and Delete RPCs (D-01),
// exercising Bindings.Merge (two constructors on one Annotations()
// slice).
func (Item) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.CreateRPC(entconnecttestv1connect.ItemCreateServiceCreateItemProcedure),
		entconnect.DeleteRPC(entconnecttestv1connect.ItemDeleteServiceDeleteItemProcedure),
	}
}

// Policy denies any mutation whose context carries a viewer.Viewer with
// Subject() == DeniedSubject, and otherwise allows — the fixture's one
// real, exercised privacy rule on the write path (T-02-09's tracer
// proof).
func (Item) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.ContextQueryMutationRule(func(ctx context.Context) error {
				v, ok := viewer.FromContext(ctx)
				if !ok {
					return privacy.Skip
				}
				if v.Subject() == DeniedSubject {
					return privacy.Deny
				}
				return privacy.Skip
			}),
			privacy.AlwaysAllowRule(),
		},
	}
}
