// Package boundarytest is the walking skeleton's real end-to-end
// verify: it runs a real entc schema-load subprocess against a real ent
// schema package declaring MixinForProto, and asserts both provenance
// annotations decode back out of the resulting *gen.Graph.
//
// No unit test can prove this boundary holds — entc.LoadGraph compiles
// and runs ./ent/schema in a subprocess (gorun(), confirmed in
// 01-RESEARCH.md by reading entc/load/load.go directly) and serializes
// the result to JSON, where load.Schema.Field.Validators is an int
// count, not closures. Annotations are the only channel across it.
// Using entc.LoadGraph here is correct and deliberate (R2): this is the
// one place in Phase 1 we want the real subprocess, because the
// subprocess's JSON boundary is exactly the risk this plan exists to
// retire. MIX-12's Validate[M] debug entry point (mixinforproto/mixin.go)
// never goes near this path.
package boundarytest

import (
	"encoding/json"
	"sort"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"

	"github.com/smintz/entconnect/mixinforproto"
)

func TestAnnotationsCrossSchemaLoadBoundary(t *testing.T) {
	graph, err := entc.LoadGraph("./ent/schema", &gen.Config{
		Target:  t.TempDir(),
		Package: "github.com/smintz/entconnect/mixinforproto/internal/boundarytest/ent",
	})
	if err != nil {
		t.Fatalf("entc.LoadGraph: %v", err)
	}

	tracerType := findType(t, graph, "Tracer")

	sm := decodeAnnotation[mixinforproto.SourceMessage](t, tracerType.Annotations, mixinforproto.MixinForProtoMessage)
	if sm.Message != "mixinforprototest.v1.Tracer" {
		t.Fatalf("want SourceMessage.Message %q, got %q", "mixinforprototest.v1.Tracer", sm.Message)
	}
	if sm.ContractVersion == 0 {
		t.Fatal("want a non-zero SourceMessage.ContractVersion — a silently-omitted version marker must fail this test")
	}
	if len(sm.Fields) != 1 || sm.Fields[0].Name != "name" || sm.Fields[0].Number != 1 {
		t.Fatalf("want field inventory [{name 1}], got %v", sm.Fields)
	}

	nameField := findField(t, tracerType, "name")

	sf := decodeAnnotation[mixinforproto.SourceField](t, nameField.Annotations, mixinforproto.MixinForProtoField)
	if sf.FieldName != "name" {
		t.Fatalf("want SourceField.FieldName %q, got %q", "name", sf.FieldName)
	}
	if sf.Number != 1 {
		t.Fatalf("want SourceField.Number=1, got %d", sf.Number)
	}
	if sf.ContractVersion == 0 {
		t.Fatal("want a non-zero SourceField.ContractVersion — a silently-omitted version marker must fail this test")
	}
}

func findType(t *testing.T, graph *gen.Graph, name string) *gen.Type {
	t.Helper()
	for _, n := range graph.Nodes {
		if n.Name == name {
			return n
		}
	}
	names := make([]string, len(graph.Nodes))
	for i, n := range graph.Nodes {
		names[i] = n.Name
	}
	sort.Strings(names)
	t.Fatalf("want a %q node in the loaded graph, got: %v", name, names)
	return nil
}

func findField(t *testing.T, typ *gen.Type, name string) *gen.Field {
	t.Helper()
	for _, f := range typ.Fields {
		if f.Name == name {
			return f
		}
	}
	names := make([]string, len(typ.Fields))
	for i, f := range typ.Fields {
		names[i] = f.Name
	}
	sort.Strings(names)
	t.Fatalf("want a %q field on %s, got: %v", name, typ.Name, names)
	return nil
}

// decodeAnnotation reads the annotation stored under key from a
// gen.Annotations map (map[string]any, populated by entc/load's own JSON
// decode of the schema-load subprocess's output) and decodes it into T
// via a stdlib encoding/json marshal/unmarshal round-trip — no
// mapstructure dependency, per this plan's module-isolation constraint.
func decodeAnnotation[T any](t *testing.T, annotations gen.Annotations, key string) T {
	t.Helper()
	var zero T
	raw, ok := annotations[key]
	if !ok {
		keys := make([]string, 0, len(annotations))
		for k := range annotations {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Fatalf("want a %q annotation, got keys: %v", key, keys)
		return zero
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("re-marshal %q annotation: %v", key, err)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("unmarshal %q annotation into %T: %v", key, v, err)
	}
	return v
}
