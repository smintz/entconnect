package entc

import (
	"encoding/json"
	"fmt"
	"sort"

	"entgo.io/ent/entc/gen"
)

// decodeAnnotation reads the annotation stored under key from a
// gen.Annotations map (map[string]any — decoded JSON, not the original
// Go annotation struct; entc/gen/graph.go's own doc comment on
// Type.Annotations confirms this) and decodes it into T via a stdlib
// encoding/json marshal/unmarshal round trip.
//
// This is the error-returning, production form of the identical idiom
// already proven against the real entc schema-load subprocess boundary
// in mixinforproto/internal/boundarytest/boundary_test.go — reused here
// rather than adding a mapstructure dependency (RESEARCH.md Pattern 6).
func decodeAnnotation[T any](annotations gen.Annotations, key string) (T, error) {
	var zero T
	raw, ok := annotations[key]
	if !ok {
		keys := make([]string, 0, len(annotations))
		for k := range annotations {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return zero, fmt.Errorf("entconnect: missing %q annotation — present keys: %v", key, keys)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return zero, fmt.Errorf("entconnect: re-marshal %q annotation: %w", key, err)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return zero, fmt.Errorf("entconnect: unmarshal %q annotation into %T: %w", key, v, err)
	}
	return v, nil
}
