// Package schema (this file): a second real ent schema type, sibling to
// tracer.go, proving D-09's SourceMessage.BoundaryOnly provenance (03-02
// Task 3) survives the same real entc schema-load subprocess boundary as
// every other annotation this package proves.
package schema

import (
	"entgo.io/ent"

	"github.com/smintz/entconnect/mixinforproto"
	mixinforprototestv1 "github.com/smintz/entconnect/mixinforproto/internal/gen/mixinforprototestv1"
)

// BoundaryOnlyFixture declares MixinForProto against the generated
// Messages message type: singular_message carries a `required` rule
// (messages.proto) and is skipped by default (never opted into AsJSON
// here, only as_json_target is), so its rule can never be enforced at
// the storage layer — SourceMessage.BoundaryOnly must record it.
type BoundaryOnlyFixture struct {
	ent.Schema
}

func (BoundaryOnlyFixture) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*mixinforprototestv1.Messages](mixinforproto.AsJSON("as_json_target")),
	}
}
