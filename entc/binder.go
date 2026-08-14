package entc

import (
	"fmt"
	"sort"

	"entgo.io/ent/schema"
)

// ContractVersion is the version of the entconnect RPC-binding
// annotation contract: the on-the-wire shape of Bindings/Binding as they
// cross entc's schema-load JSON boundary. Mirrors mixinforproto's own
// ContractVersion precedent (mixinforproto/annotation.go) — bumped only
// on a breaking layout change, additive-only between bumps.
const ContractVersion = 1

// RPCBindings is the schema.Annotation key under which Bindings is
// stored on gen.Type.Annotations. Exported so downstream decoders
// (this package's own decode.go, Phase 5's drift check) reference the
// constant rather than re-typing the literal (mirrors entproto's
// MessageAnnotation precedent, and mixinforproto.MixinForProtoMessage).
const RPCBindings = "EntconnectRPCBindings"

// Op names one CRUD operation (or the Manual escape hatch) a Binding
// claims a procedure for.
type Op string

// The six Op values GetRPC/ListRPC/CreateRPC/UpdateRPC/DeleteRPC/Manual
// each construct.
const (
	OpGet    Op = "get"
	OpList   Op = "list"
	OpCreate Op = "create"
	OpUpdate Op = "update"
	OpDelete Op = "delete"
	OpManual Op = "manual"
)

// Binding is one RPC claim: an Op and the Connect procedure string it
// binds ("/pkg.Service/Method"). OrderBy is populated only by ListRPC —
// D-09's schema-side default ordering channel — and is empty for every
// other Op.
type Binding struct {
	Op        Op       `json:"op"`
	Procedure string   `json:"procedure"`
	OrderBy   []string `json:"orderBy,omitempty"`
}

// Bindings is the schema-level RPC-binding provenance annotation written
// by GetRPC/ListRPC/CreateRPC/UpdateRPC/DeleteRPC/Manual in a schema's
// Annotations() method. It is a plain, JSON-serializable struct — no
// closures, no interface-typed fields — following mixinforproto.SourceMessage's
// exact precedent (D-02), because this is the only channel across entc's
// schema-load JSON boundary for RPC-binding provenance.
type Bindings struct {
	ContractVersion int       `json:"contractVersion"`
	Bindings        []Binding `json:"bindings"`
}

// Name implements entgo.io/ent/schema.Annotation.
func (Bindings) Name() string {
	return RPCBindings
}

// Merge implements entgo.io/ent/schema.Merger. Ent's schema-load
// pipeline (entc/load/schema.go addAnnotation) calls this whenever a
// second Bindings-named annotation is added to the same schema — which
// is exactly what happens every time an adopter writes more than one
// GetRPC/CreateRPC/... in one Annotations() slice, since each
// constructor below returns its own single-entry Bindings value.
// Collapses the two into one Bindings holding the union of both
// Bindings slices, sorted by Op then Procedure so the merged value is
// order-independent (D-20: repeated schema-load runs must produce the
// byte-identical merged annotation regardless of Annotations() call
// order).
func (b Bindings) Merge(other schema.Annotation) schema.Annotation {
	o, ok := other.(Bindings)
	if !ok {
		return b
	}
	merged := Bindings{
		ContractVersion: ContractVersion,
		Bindings:        append(append([]Binding{}, b.Bindings...), o.Bindings...),
	}
	sort.SliceStable(merged.Bindings, func(i, j int) bool {
		if merged.Bindings[i].Op != merged.Bindings[j].Op {
			return merged.Bindings[i].Op < merged.Bindings[j].Op
		}
		return merged.Bindings[i].Procedure < merged.Bindings[j].Procedure
	})
	return merged
}

// single builds a one-entry Bindings for op/procedure, validating that
// orderBy is only ever populated for OpList — every non-list constructor
// below passes nil.
func single(op Op, procedure string, orderBy []string) Bindings {
	if op != OpList && len(orderBy) > 0 {
		panic(fmt.Sprintf("entconnect: %s binding for %q must not carry an orderBy — orderBy is ListRPC-only (D-09)", op, procedure))
	}
	return Bindings{
		ContractVersion: ContractVersion,
		Bindings: []Binding{{
			Op:        op,
			Procedure: procedure,
			OrderBy:   orderBy,
		}},
	}
}

// GetRPC binds procedure (a connect-go generated "…Procedure" constant,
// D-01) as this schema's Get RPC.
func GetRPC(procedure string) Bindings { return single(OpGet, procedure, nil) }

// CreateRPC binds procedure as this schema's Create RPC.
func CreateRPC(procedure string) Bindings { return single(OpCreate, procedure, nil) }

// UpdateRPC binds procedure as this schema's Update RPC.
func UpdateRPC(procedure string) Bindings { return single(OpUpdate, procedure, nil) }

// DeleteRPC binds procedure as this schema's Delete RPC.
func DeleteRPC(procedure string) Bindings { return single(OpDelete, procedure, nil) }

// Manual binds procedure to a hand-written handler the application
// supplies (D-06). A Manual RPC still runs inside the generated
// interceptor chain — the generated wiring registers it on the same
// <Service>Handler interface as every CRUD method, so it inherits the
// identical authn/viewer/protovalidate/otel chain (INT-04).
func Manual(procedure string) Bindings { return single(OpManual, procedure, nil) }

// ListRPC binds procedure as this schema's List RPC. orderBy is the
// schema-side default ordering channel D-09 requires ("ordering comes
// from a schema-side default, not from the contract"): each entry names
// a proto field (in SourceField.FieldName form) to order by, applied in
// the given order. An empty orderBy means "order by the entity's primary
// key alone".
func ListRPC(procedure string, orderBy ...string) Bindings {
	return single(OpList, procedure, orderBy)
}

var _ schema.Annotation = Bindings{}
var _ schema.Merger = Bindings{}
