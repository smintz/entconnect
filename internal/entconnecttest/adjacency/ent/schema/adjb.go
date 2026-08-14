// Package schema (this file) is adja.go's sibling half of this in-test
// adjacency fixture — see that file's own doc comment.
package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"

	entconnect "github.com/smintz/entconnect/entc"
	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"
	"github.com/smintz/entconnect/mixinforproto"
)

// AdjacencyPing claims AdminService's Ping RPC — the second of the two
// schemas whose bindings target the same proto service as
// AdjacencyArchive.
type AdjacencyPing struct {
	ent.Schema
}

func (AdjacencyPing) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.Admin](mixinforproto.Exclude("id")),
	}
}

func (AdjacencyPing) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.Manual(entconnecttestv1connect.AdminServicePingProcedure),
	}
}
