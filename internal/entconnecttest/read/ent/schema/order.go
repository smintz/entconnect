// Package schema is the walking-slice fixture's real ent schema package —
// the entc extension's write-side proof (mirroring
// mixinforproto/internal/boundarytest's own real-entc-schema-load
// harness, which only proves the read side).
package schema

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/privacy"
	entschema "entgo.io/ent/schema"

	entconnect "github.com/smintz/entconnect/entc"
	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"
	"github.com/smintz/entconnect/mixinforproto"
	"github.com/smintz/entconnect/runtime/viewer"
)

// DeniedSubject is a sentinel viewer.Viewer.Subject() value the fixture's
// privacy policy below denies — tracer_test.go uses it to prove a real
// ent privacy denial surfaces as Connect's PermissionDenied (INT-03).
const DeniedSubject = "denied-subject"

// Order declares MixinForProto against the generated Order message type
// (D-01: no committed descriptor file, no string message name anywhere
// in the call) and binds OrderReadService's GetOrder RPC via
// entconnect.GetRPC — a compile-time-checked reference to
// entconnecttestv1connect's generated procedure constant.
type Order struct {
	ent.Schema
}

// Mixin derives customer/status/created_at from the Order message.
// "id" is excluded: it collides with ent's own reserved, per-type "id"
// identifier (mixinforproto/reserved.go's reservedStructural), so this
// entity uses ent's own default auto-incrementing int ID instead — the
// generated GetOrder handler converts between the two at the boundary.
func (Order) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.Order](mixinforproto.Exclude("id")),
	}
}

// Annotations binds this schema's Get RPC (D-01).
func (Order) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.GetRPC(entconnecttestv1connect.OrderReadServiceGetOrderProcedure),
	}
}

// Policy denies any query whose context carries a viewer.Viewer with
// Subject() == DeniedSubject, and otherwise allows — the fixture's one
// real, exercised privacy rule (T-02-03/INT-03's tracer proof).
func (Order) Policy() ent.Policy {
	return privacy.Policy{
		Query: privacy.QueryPolicy{
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
