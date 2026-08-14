// Package schema is the Manual-escape-hatch fixture's real ent schema
// package — mirrors internal/entconnecttest/read/ent/schema's shape,
// binding one procedure through entconnect.Manual instead of a CRUD
// constructor (D-06/INT-04).
package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"

	entconnect "github.com/smintz/entconnect/entc"
	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"
	"github.com/smintz/entconnect/mixinforproto"
)

// Admin declares MixinForProto against the generated Admin message type
// (D-01: no committed descriptor file, no string message name anywhere in
// the call) and binds AdminService's ArchiveAdmin RPC through
// entconnect.Manual — a compile-time-checked reference to
// entconnecttestv1connect's generated procedure constant, exactly like
// every CRUD binder (D-06). AdminService's second RPC, Ping, is
// deliberately left unclaimed by any schema (see admin.proto), so
// entc/claims.go's report has a real "unclaimed" row and the generated
// wiring's own compile-time obligation for an unclaimed method on a
// partly-claimed service has a real second method to prove itself
// against.
type Admin struct {
	ent.Schema
}

// Mixin derives label from the Admin message. "id" is excluded: it
// collides with ent's own reserved, per-type "id" identifier
// (mixinforproto/reserved.go's reservedStructural), so this entity uses
// ent's own default auto-incrementing int ID instead. The manual
// ArchiveAdmin handler never touches the ent client at all, so this
// derivation exists only to give Admin a real, checkable MixinForProto
// provenance annotation — every RPC-binding schema carries one,
// regardless of Op.
func (Admin) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.Admin](mixinforproto.Exclude("id")),
	}
}

// Annotations binds this schema's ArchiveAdmin RPC through the Manual
// escape hatch (D-06). Ping is intentionally never bound here.
func (Admin) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.Manual(entconnecttestv1connect.AdminServiceArchiveAdminProcedure),
	}
}
