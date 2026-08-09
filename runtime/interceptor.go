package runtime

import (
	"context"
	"errors"
	"net/http"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"connectrpc.com/validate"

	"github.com/smintz/entconnect/runtime/viewer"
)

// Authenticator is the single-method interface generated server wiring
// requires as a required positional constructor argument (D-11): an
// application implements it once, threading its own auth mechanism
// (a bearer token, a session cookie, ...) into a viewer-scoped context.
// Because it is required and positional (never a functional option),
// "forgot to wire authn" is a compile error rather than an open
// endpoint — the property T-02-01 in the phase threat model relies on.
type Authenticator interface {
	// AuthenticateAndViewer authenticates the caller from header and
	// returns a context carrying a viewer.Viewer (see runtime/viewer),
	// or an error if authentication fails. Returning ctx unchanged (no
	// viewer attached) is treated by ViewerInterceptor as a missing
	// viewer, not as an anonymous allow (INT-02).
	AuthenticateAndViewer(ctx context.Context, header http.Header) (context.Context, error)
}

// AuthnInterceptor calls a.AuthenticateAndViewer for every request,
// mapping a returned error through MapError and otherwise continuing
// with the context AuthenticateAndViewer returned.
func AuthnInterceptor(a Authenticator) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			authedCtx, err := a.AuthenticateAndViewer(ctx, req.Header())
			if err != nil {
				return nil, MapError(err)
			}
			return next(authedCtx, req)
		}
	})
}

// ViewerInterceptor asserts the context reaching it carries a
// viewer.Viewer (placed there by the preceding authn stage) and refuses
// with CodeUnauthenticated when it does not — INT-02's core guarantee: a
// missing viewer is never treated as an anonymous allow.
func ViewerInterceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if _, ok := viewer.FromContext(ctx); !ok {
				return nil, connect.NewError(connect.CodeUnauthenticated, errMissingViewer)
			}
			return next(ctx, req)
		}
	})
}

// errMissingViewer is INT-02's refusal reason: a context reaching the
// viewer-injection stage with no viewer attached is refused, never
// treated as an anonymous allow.
var errMissingViewer = errors.New("entconnect: request context carries no viewer")

// Chain builds the fixed authn -> viewer injection -> protovalidate ->
// otel interceptor chain (INT-01) as a single connect.Option, applying
// opts first to determine the protovalidate validator and otel
// interceptor instances used. connect.WithInterceptors composes
// first-listed-outermost (connectrpc.com/connect@v1.20.0/option.go's own
// doc comment, verified in RESEARCH.md Pattern 4), which is exactly
// INT-01's required order — authn runs first on the request and last on
// the response.
//
// Chain is called exactly once per process by generated server wiring
// (D-13): the protovalidate validator and otel interceptor it builds are
// shared across every concurrent request from then on, never rebuilt
// per-request.
func Chain(a Authenticator, opts ...Option) (connect.Option, error) {
	cfg, err := newConfig(opts...)
	if err != nil {
		return nil, err
	}
	var validateOpts []validate.Option
	if cfg.validator != nil {
		validateOpts = append(validateOpts, validate.WithValidator(cfg.validator))
	}
	validateInterceptor := validate.NewInterceptor(validateOpts...)
	return connect.WithInterceptors(
		AuthnInterceptor(a),
		ViewerInterceptor(),
		validateInterceptor,
		cfg.otel,
	), nil
}

// config is the accumulated, unexported configuration Option values
// build. Unexported deliberately (D-11's "narrow Option" requirement):
// an application can only ever set what a runtime-provided With* function
// exposes, never add/remove/reorder a chain stage.
type config struct {
	validator protovalidate.Validator
	otel      *otelconnect.Interceptor
}

// Option configures the otel interceptor instance and/or the
// protovalidate validator instance Chain builds. It may NOT add, remove,
// or reorder chain stages.
type Option func(*config) error

// WithValidator overrides the protovalidate.Validator Chain uses. When
// omitted, connectrpc.com/validate's own default
// (protovalidate.GlobalValidator, built lazily and cached — already
// satisfying D-13's "once per process") is used.
func WithValidator(v protovalidate.Validator) Option {
	return func(c *config) error {
		c.validator = v
		return nil
	}
}

// WithOtelInterceptor overrides the *otelconnect.Interceptor Chain uses.
// When omitted, Chain builds one via otelconnect.NewInterceptor() with no
// options.
func WithOtelInterceptor(i *otelconnect.Interceptor) Option {
	return func(c *config) error {
		c.otel = i
		return nil
	}
}

func newConfig(opts ...Option) (*config, error) {
	cfg := &config{}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	if cfg.otel == nil {
		otelInterceptor, err := otelconnect.NewInterceptor()
		if err != nil {
			return nil, err
		}
		cfg.otel = otelInterceptor
	}
	return cfg, nil
}
