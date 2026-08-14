// Package schema is entc/extension_test.go's minimal, in-test-only "zero
// bindings" fixture (CRUD-06's empty edge): a real ent schema with no
// entconnect annotations at all, proving a graph with zero entconnect
// bindings emits zero handler files while the claims report is still
// written. Never go:generate'd for real, never committed as generated
// output: loaded only via entc.LoadGraph inside
// entc/extension_test.go's TestGolden_Empty.
package schema

import "entgo.io/ent"

// Nothing declares no Mixin and no Annotations — Generate has nothing to
// do for it at all.
type Nothing struct {
	ent.Schema
}
