// Package schema is the hookwiring fixture's real ent schema package —
// mirrors internal/entconnecttest/{update,write}/ent/schema's shape. This
// file declares the single marker-recording mechanism VAL-09's relative
// hook-ordering assertion (ordering_test.go) observes: a *Recorder carried
// on the mutation context, appended to by Policed's Policy() and by both
// schemas' schema-declared Hooks(). The test asserts on the recorded
// sequence, never on an index into ent's own hooks slice — see
// policed.go's package doc for why.
package schema

import (
	"context"
	"sync"
)

// Recorder accumulates ordered markers ("policy", "schema-hook") across a
// single mutation's Policy/Hooks evaluation. A pointer, not a value: ent's
// generated privacy/hook wiring never copies the context value it reads,
// so every append below lands in the same backing slice the test itself
// holds a reference to. Guarded by mu because privacy rules and hooks can,
// in general, run concurrently across independent mutations sharing one
// process — this fixture's own tests run one mutation at a time, but the
// guard costs nothing and removes the need to document that assumption as
// a precondition of correctness.
type Recorder struct {
	mu      sync.Mutex
	markers []string
}

// Append adds marker to r in call order.
func (r *Recorder) Append(marker string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markers = append(r.markers, marker)
}

// Markers returns a copy of the recorded sequence so a caller cannot
// mutate r's internal slice through the returned value.
func (r *Recorder) Markers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.markers))
	copy(out, r.markers)
	return out
}

// recorderCtxKey is the unexported context-key type NewRecorderContext/
// RecorderFromContext use — an unexported type per context.WithValue's own
// documented convention, avoiding collisions with any other package's
// context key.
type recorderCtxKey struct{}

// NewRecorderContext returns a context carrying r, retrievable by
// RecorderFromContext — both Policed.Policy and {Policed,Unpoliced}.Hooks
// read it via this exact accessor pair.
func NewRecorderContext(ctx context.Context, r *Recorder) context.Context {
	return context.WithValue(ctx, recorderCtxKey{}, r)
}

// RecorderFromContext retrieves the *Recorder NewRecorderContext attached
// to ctx, if any. A context carrying no recorder (any real, non-test
// mutation path) reports ok == false — Policy/Hooks below treat that as
// "nothing to record", never as an error.
func RecorderFromContext(ctx context.Context) (*Recorder, bool) {
	r, ok := ctx.Value(recorderCtxKey{}).(*Recorder)
	return r, ok
}
