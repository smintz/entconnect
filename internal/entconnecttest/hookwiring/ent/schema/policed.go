// Package schema — see recorder.go's package doc.
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

// DeniedSubject is a sentinel viewer.Viewer.Subject() value Policed's
// Policy denies outright (Test 5: a denying policy on a mutation that ALSO
// violates a constraint still yields privacy.Deny, never a
// *protovalidate.ValidationError — there is no constraint-existence leak
// to an unauthorized caller).
const DeniedSubject = "denied-subject"

// Policed declares MixinForProto against the same HookWiring message type
// as Unpoliced (unpoliced.go) and additionally declares a Policy() — the
// two schemas are byte-identical apart from that one method, which is the
// whole point: 03-RESEARCH.md Pitfall 3 established that ent's generated
// wiring installs the combined privacy Policy at Hooks[0] only when SOME
// Policy() exists anywhere on the schema, so a hard-coded "hook index 0 is
// the mixin hook" assertion would silently flip from true to false the
// moment an app adds a Policy() — turning an app-shaped change into a
// false regression. ordering_test.go asserts the RELATIVE sequence
// "privacy (when declared) -> mixin hook -> schema hooks" in both this
// schema and Unpoliced, never an absolute index.
//
// Policed also binds HookWiringCreateService's CreateHookWiring RPC (D-01)
// — the real Connect surface wiring_test.go drives for D-13's real-client
// wiring proof; Unpoliced needs no RPC binding since ordering_test.go
// drives it directly through its generated ent.Client.
type Policed struct {
	ent.Schema
}

// Mixin derives cel_field/standard_field from the HookWiring message. "id"
// is excluded: it collides with ent's own reserved, per-type "id"
// identifier (mixinforproto/reserved.go's reservedStructural), exactly as
// every other fixture in this repo excludes it — this entity uses ent's
// own default auto-incrementing int ID instead.
func (Policed) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixinforproto.MixinForProto[*entconnecttestv1.HookWiring](mixinforproto.Exclude("id")),
	}
}

// Annotations binds this schema's Create RPC (D-01).
func (Policed) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.CreateRPC(entconnecttestv1connect.HookWiringCreateServiceCreateHookWiringProcedure),
	}
}

// Policy records a "policy" marker on the mutation context's *Recorder (if
// any) for every mutation, then denies outright when the context's viewer
// has Subject() == DeniedSubject, and otherwise allows — mirroring
// internal/entconnecttest/{update,write}'s existing DeniedSubject
// convention. The marker is recorded unconditionally, BEFORE the
// deny/allow decision, matching ent's real behavior: the combined privacy
// Policy always runs (and this rule always observes the mutation) whether
// or not it ultimately denies — recording after would wrongly suggest a
// denied caller left no trace, which is not the property Test 5 asserts
// (Test 5 asserts the returned error's TYPE, not marker presence).
func (Policed) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.ContextQueryMutationRule(func(ctx context.Context) error {
				if r, ok := RecorderFromContext(ctx); ok {
					r.Append("policy")
				}
				if v, ok := viewer.FromContext(ctx); ok && v.Subject() == DeniedSubject {
					return privacy.Deny
				}
				return privacy.Skip
			}),
			privacy.AlwaysAllowRule(),
		},
	}
}

// Hooks records a "schema-hook" marker — the schema-declared hook
// ordering_test.go proves runs strictly AFTER the mixin's own validation
// hook (an invalid mutation never reaches this hook at all: mixinforproto's
// Hooks() entry rejects it first and returns without calling next.Mutate).
func (Policed) Hooks() []ent.Hook {
	return []ent.Hook{
		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				if r, ok := RecorderFromContext(ctx); ok {
					r.Append("schema-hook")
				}
				return next.Mutate(ctx, m)
			})
		},
	}
}
