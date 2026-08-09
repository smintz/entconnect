// Package viewer is entconnect's first-party viewer type and context
// carrier. Core entgo.io/ent ships no canonical viewer type of its own —
// ent's privacy tutorial uses one extensively, but it lives only in
// entgo.io/ent/examples/privacyadmin/viewer, an example-app package never
// meant to be imported cross-module. This package fills that gap so
// entconnect's generated interceptor chain (runtime.ViewerInterceptor)
// and an application's own ent privacy.Policy rules have a shared,
// first-party type to agree on.
package viewer

import "context"

// Viewer identifies the caller a request's ent privacy.Policy rules
// evaluate against. Applications implement this from whatever their
// Authenticator resolves (a user record, a service identity, ...).
type Viewer interface {
	// Subject returns a stable identifier for the caller (a user ID, a
	// service account name, ...).
	Subject() string
	// Roles returns the caller's roles, for privacy rules that gate on
	// role membership (e.g. AllowIfAdmin-style policies).
	Roles() []string
}

// ctxKey is unexported so no other package can construct a colliding
// context key.
type ctxKey struct{}

// NewContext returns a copy of parent carrying v as the request's
// viewer.
func NewContext(parent context.Context, v Viewer) context.Context {
	return context.WithValue(parent, ctxKey{}, v)
}

// FromContext returns the Viewer previously attached via NewContext, and
// whether one was present. A context with no viewer attached is a
// distinct, checkable condition — runtime.ViewerInterceptor uses exactly
// this to refuse rather than treat a missing viewer as anonymous-allow
// (INT-02).
func FromContext(ctx context.Context) (Viewer, bool) {
	v, ok := ctx.Value(ctxKey{}).(Viewer)
	return v, ok
}
