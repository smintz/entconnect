package mixinforproto

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/mixin"
	"google.golang.org/protobuf/proto"
)

// protoMixin is the ent.Mixin adapter MixinForProto returns. It embeds
// mixin.Schema so every ent.Mixin method it does not override (Edges,
// Indexes, Interceptors, Policy) is satisfied with mixin.Schema's
// correct no-op default. Hooks() is overridden below (Phase 3, VAL-04):
// Phase 1 shipped no hook (D-27's ordering note documented this ahead of
// Phase 3 actually adding one); this is that hook.
type protoMixin[M proto.Message] struct {
	mixin.Schema
	opts []Option
}

// MixinForProto derives an ent.Mixin from a generated protobuf message
// type M. Declared in a schema's Mixin() method, it materializes one
// ent.Field per contract field at schema-load time — no committed
// descriptor file, no string message name anywhere in the call (MIX-01):
//
//	func (Order) Mixin() []ent.Mixin {
//		return []ent.Mixin{
//			mixinforproto.MixinForProto[*orderv1.Order](),
//		}
//	}
func MixinForProto[M proto.Message](opts ...Option) ent.Mixin {
	return &protoMixin[M]{opts: opts}
}

// Fields implements ent.Mixin. Every failure path inside derive is an
// ordinary Go error; panicking is isolated to this one adapter method
// (D-06), which is what makes Validate[M] nearly free instead of a
// parallel code path.
func (m *protoMixin[M]) Fields() []ent.Field {
	d, err := derive[M](m.opts...)
	if err != nil {
		panic(err)
	}
	return d.fields
}

// Annotations implements ent.Mixin. It runs the identical derive core
// used by Fields and Validate, and returns the schema-level provenance
// annotation — the only channel across entc's schema-load JSON boundary
// (ANNO-01). ent.Mixin.Annotations()'s own doc comment confirms this
// return value is added to the schema's own annotations, the same
// channel a schema author's hand-written Annotations() method would use.
func (m *protoMixin[M]) Annotations() []schema.Annotation {
	d, err := derive[M](m.opts...)
	if err != nil {
		panic(err)
	}
	return []schema.Annotation{d.message}
}

// Hooks implements ent.Mixin (VAL-04). It compiles this plan's residual
// custom-CEL evaluation state once, at schema-load time (buildHookState,
// hooks.go), and panics — the same panicking-adapter shape Fields()
// above uses — if a residual CEL expression fails to compile (D-09), or
// (Plan 03-05, VAL-08/D-10) if m.opts carries WithMessageRules(OnCreate)
// and a message-level rule references a field this package cannot
// reconstruct — excluded, overridden, or underivable
// (messagerules.go's checkMessageRuleReferences). A message with no
// residual CEL rules, no standard rules, and no message-level rules at
// all gets no hook: len(evaluators) == 0 returns nil, so mixinforproto
// adds zero mutation-time cost for a contract that carries none.
func (m *protoMixin[M]) Hooks() []ent.Hook {
	md := (*new(M)).ProtoReflect().Descriptor()
	hs, err := buildHookState(md, m.opts...)
	if err != nil {
		panic(err)
	}
	if len(hs.evaluators) == 0 {
		return nil
	}
	return []ent.Hook{hs.hook()}
}

// Validate is MIX-12's in-process debug entry point (D-07): it calls the
// identical derive core directly and returns the structured error rather
// than panicking, so a developer can call it from a plain `go test` in
// their own schema package and get a full, untruncated error.
//
// Validate never touches entc.LoadGraph or anything under entc/load, so
// it never enters entc's gorun() subprocess (R2) — entc.LoadGraph itself
// still shells out through that same subprocess and cannot serve as this
// debug entry point; only a function that bypasses entc/load entirely
// does.
func Validate[M proto.Message](opts ...Option) error {
	_, err := derive[M](opts...)
	return err
}
