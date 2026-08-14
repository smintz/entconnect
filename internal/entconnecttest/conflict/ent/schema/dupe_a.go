// Package schema is entc/claims_test.go's negative-build fixture for
// D-05: DupeA (this file) and DupeB (dupe_b.go) both claim the SAME
// procedure (OrderReadServiceGetOrderProcedure) via GetRPC, proving a
// procedure claimed by two schemas is a collected, build-fatal conflict —
// never a silent first-claimant-wins. DupeA additionally declares GetRPC
// twice on itself, against two DIFFERENT procedures, proving the sibling
// D-05 conflict shape (a schema declaring the same Op twice) is collected
// in the SAME pass, not masked by the first failure. Loaded via
// entc.LoadGraph in isolation from every other ent/schema package in this
// repo — these are real Go packages that compile; only
// entconnect.Generate is expected to fail on them.
package schema

import (
	"entgo.io/ent"
	entschema "entgo.io/ent/schema"

	entconnect "github.com/smintz/entconnect/entc"
	"github.com/smintz/entconnect/internal/gen/entconnecttestv1/entconnecttestv1connect"
)

// DupeA declares GetRPC twice against two distinct procedures
// (OrderReadService/GetOrder and PageListService/ListPages) — the
// duplicate-op conflict shape — and its first binding collides with
// DupeB's own GetRPC binding on the same procedure — the duplicate-claim
// conflict shape.
type DupeA struct {
	ent.Schema
}

func (DupeA) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		entconnect.GetRPC(entconnecttestv1connect.OrderReadServiceGetOrderProcedure),
		entconnect.GetRPC(entconnecttestv1connect.PageListServiceListPagesProcedure),
	}
}
