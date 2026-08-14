// Package schema (this file) is badpatch.go's sibling negative-build
// fixture: a schema declaring MixinForProto with an Exclude of a field
// the update surface still needs, proving CRUD-05/D-17's "excluded"
// classification is a real, distinct build failure.
package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"

	entconnect "github.com/smintz/entconnect/entc"
	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"
	"github.com/smintz/entconnect/mixinforproto"
)

// ExcludedPatch binds PatchUpdateService's real UpdatePatch procedure —
// distinct from badpatch.go's BadPatch, which binds UpdatePatchAlt, so
// the two negative fixtures never collide under D-05's duplicate-claim
// check when loaded together in this package's one entc.LoadGraph call
// (badmask's graph is loaded in isolation from
// internal/entconnecttest/update/ent/schema's own real UpdatePatch
// binding, so reusing that same procedure string here is safe).
//
// "body" is Exclude()d even though nothing else marks it as
// deliberately off the Update surface — ValidateMaskPaths classifies
// this as "excluded": a field present on the entity descriptor and in
// SourceMessage.Excluded. Unlike "internal_note" (which the real,
// working Patch fixture also excludes, by design, and is never
// regenerated again after this plan wires ValidateMaskPaths in — see
// 02-04-SUMMARY.md's Deviations section), this fixture exists purely to
// prove the classification fires and is text-distinguishable from the
// "unknown" case.
type ExcludedPatch struct {
	ent.Schema
}

func (ExcludedPatch) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.Patch](
			mixinforproto.Exclude("id", "internal_note", "body"),
		),
	}
}

func (ExcludedPatch) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.UpdateRPC(entconnecttestv1connect.PatchUpdateServiceUpdatePatchProcedure),
	}
}
