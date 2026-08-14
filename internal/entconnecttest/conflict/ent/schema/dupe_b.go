// Package schema (this file) is dupe_a.go's sibling half of this
// negative-build fixture — see that file's own doc comment.
package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"

	entconnect "github.com/smintz/entconnect/entc"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"
)

// DupeB claims the same procedure DupeA's first binding does
// (OrderReadService/GetOrder), the duplicate-claim conflict shape D-05
// requires.
type DupeB struct {
	ent.Schema
}

func (DupeB) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.GetRPC(entconnecttestv1connect.OrderReadServiceGetOrderProcedure),
	}
}
