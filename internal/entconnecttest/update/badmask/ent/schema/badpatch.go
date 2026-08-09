// Package schema is a negative-build fixture (entc/maskcheck_test.go):
// a schema whose UpdateRPC binding names a message with a field the
// derived set cannot satisfy, proving CRUD-05/D-17's "unknown"
// classification is a real build failure. Loaded via entc.LoadGraph in
// isolation from internal/entconnecttest/update/ent/schema — never
// go:generate'd for real (see maskcheck_test.go's own header comment).
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	entschema "entgo.io/ent/schema"

	entconnect "github.com/smintz/entconnect/entc"
	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"
	"github.com/smintz/entconnect/mixinforproto"
)

// BadPatch binds PatchUpdateAltService's UpdatePatchAlt RPC (a
// procedure distinct from the real fixture's UpdatePatch — see
// update.proto's own PatchUpdateAltService doc comment — so this
// negative fixture's binding never collides with excludedpatch.go's own
// binding under D-05's duplicate-claim check when both are loaded
// together).
//
// "revision" is Override()'d with a hand-declared ent.Field carrying no
// SourceField provenance (mixinforproto/option.go's documented
// behavior: Override installs the replacement verbatim, with no
// derivation annotation attached) — so entc/crud_update.go's allowed-
// set computation, which reads SourceField.FieldName off each derived
// field's annotation, can never resolve "revision" to a settable ent
// field. ValidateMaskPaths classifies this as "unknown": a field
// present on the entity descriptor and not excluded, but with no
// corresponding settable ent field.
type BadPatch struct {
	ent.Schema
}

func (BadPatch) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.Patch](
			mixinforproto.Exclude("id", "internal_note"),
			mixinforproto.Override("revision", field.Int("revision")),
		),
	}
}

func (BadPatch) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.UpdateRPC(entconnecttestv1connect.PatchUpdateAltServiceUpdatePatchAltProcedure),
	}
}
