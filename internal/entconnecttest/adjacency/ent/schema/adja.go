// Package schema is entc/extension_test.go's minimal, in-test-only
// adjacency fixture (CRUD-06's "two schemas, one service" edge — the
// plan's own sanctioned fallback, since every other fixture in this repo
// owns a single, distinct proto service and would need editing to
// overlap). Never go:generate'd for real, never committed as generated
// output: loaded only via entc.LoadGraph inside
// entc/extension_test.go's TestGolden_Adjacency.
package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"

	entconnect "github.com/smintz/entconnect/entc"
	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"
	"github.com/smintz/entconnect/mixinforproto"
)

// AdjacencyArchive claims AdminService's ArchiveAdmin RPC through the
// Manual escape hatch — one of two schemas whose bindings both target
// methods on the SAME proto service (AdminService), proving CRUD-06's
// adjacency edge: two schemas' contributions land in one emitted file, in
// ascending method order, neither overwriting the other.
type AdjacencyArchive struct {
	ent.Schema
}

func (AdjacencyArchive) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.Admin](mixinforproto.Exclude("id")),
	}
}

func (AdjacencyArchive) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.Manual(entconnecttestv1connect.AdminServiceArchiveAdminProcedure),
	}
}
