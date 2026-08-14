// Package schema is the FieldMask-gated update fixture's real ent schema
// package — mirrors internal/entconnecttest/read/ent/schema's own
// real-entc-schema-load harness shape (02-01).
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
// privacy policy below denies — mirrors internal/entconnecttest/read's
// Order.DeniedSubject (02-01), proving a real ent privacy denial on a
// masked Update surfaces as Connect's PermissionDenied (T-02-22).
const DeniedSubject = "denied-subject"

// Patch declares MixinForProto against the generated Patch message type
// and binds PatchUpdateService's UpdatePatch RPC via entconnect.UpdateRPC
// — a compile-time-checked reference to entconnecttestv1connect's
// generated procedure constant (D-01).
type Patch struct {
	ent.Schema
}

// Mixin derives title/body/revision from the Patch message.
//
//   - "id" is excluded: it collides with ent's own reserved, per-type
//     "id" identifier (mixinforproto/reserved.go's reservedStructural),
//     exactly as internal/entconnecttest/read's Order schema does — this
//     entity uses ent's own default auto-incrementing int ID instead.
//   - "internal_note" is deliberately excluded: it is never meant to be
//     settable through the Update RPC at all, proving CRUD-05/D-17's
//     "deliberately excluded" field never gets a generated Set case.
func (Patch) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.Patch](mixinforproto.Exclude("id", "internal_note")),
	}
}

// Annotations binds this schema's Update RPC (D-01).
func (Patch) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.UpdateRPC(entconnecttestv1connect.PatchUpdateServiceUpdatePatchProcedure),
	}
}

// Policy denies any mutation whose context carries a viewer.Viewer with
// Subject() == DeniedSubject, and otherwise allows — mirrors
// internal/entconnecttest/read's Order.Policy (02-01), but as a
// MutationPolicy since Update is a mutation, not a query (T-02-22).
func (Patch) Policy() ent.Policy {
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
