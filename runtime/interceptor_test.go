package runtime

// This file is 03-04's VAL-10 test: Phase 2 (D-13) already built
// once-per-process protovalidate.Validator construction into runtime.Chain
// (see Chain's own doc comment above); this phase owes the TEST that pins
// it. Three properties are asserted:
//
//  1. protovalidate.GlobalValidator (the default Chain uses when no
//     WithValidator override is given) is a fixed package-level value —
//     reading it twice always yields the identical Validator.
//  2. A runtime.Chain built once and driven through several SEQUENTIAL
//     requests never constructs a new validator on the request path — a
//     counting WithValidator implementation's construction counter stays
//     at exactly 1 no matter how many requests flow through.
//  3. The same holds under CONCURRENCY: several goroutines concurrently
//     building Chains and issuing requests through a shared
//     WithValidator-supplied instance still observe exactly one
//     construction, and every Validate call is race-safe (go test -race).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/smintz/entconnect/runtime/viewer"
)

// alwaysAllowAuthenticator is a test-only Authenticator attaching a fixed
// viewer to every request — the interceptor tests below care about the
// protovalidate stage, not authn/viewer behavior (already covered by
// Phase 2's own tests).
type alwaysAllowAuthenticator struct{}

func (alwaysAllowAuthenticator) AuthenticateAndViewer(ctx context.Context, _ http.Header) (context.Context, error) {
	return viewer.NewContext(ctx, testViewer{subject: "test-subject"}), nil
}

type testViewer struct{ subject string }

func (v testViewer) Subject() string { return v.subject }
func (v testViewer) Roles() []string { return nil }

// countingValidator is a protovalidate.Validator whose CONSTRUCTION is
// counted separately from its Validate CALLS. newCountingValidator
// increments the package-level constructionCount exactly once per call —
// VAL-10's assertion is that this counter stays at 1 for the lifetime of
// one runtime.Chain no matter how many requests that Chain serves, proving
// no validator is ever constructed on a request path. calls is incremented
// on every Validate invocation, proving validation genuinely ran once per
// request rather than being silently skipped. Always accepts (returns
// nil): this file tests CONSTRUCTION/IDENTITY behavior, not rejection
// behavior (already covered by runtime/errormap_test.go and the hookwiring
// wiring proof).
type countingValidator struct {
	calls *atomic.Int64
}

var constructionCount atomic.Int64

// newCountingValidator constructs exactly one countingValidator, recording
// the construction in the package-level constructionCount. Call this
// EXACTLY ONCE per test (before any Chain is built or any goroutine is
// spawned) — the whole point of the tests below is that nothing else in
// runtime.Chain's request path ever calls this a second time.
func newCountingValidator(calls *atomic.Int64) *countingValidator {
	constructionCount.Add(1)
	return &countingValidator{calls: calls}
}

func (v *countingValidator) Validate(_ proto.Message, _ ...protovalidate.ValidationOption) error {
	v.calls.Add(1)
	return nil
}

var _ protovalidate.Validator = (*countingValidator)(nil)

// echoProcedure is a bare, hand-declared Connect procedure this file uses
// to exercise runtime.Chain's built connect.Option without depending on
// any generated service — wrapperspb.StringValue carries no protovalidate
// rule at all, so every request this file sends is valid and the
// interceptor chain's protovalidate stage always calls through to
// next(ctx, req) after validating.
const echoProcedure = "/entconnect.runtime.internal.EchoService/Echo"

// newEchoServer mounts a raw connect.NewUnaryHandler wrapped in chainOpt
// (the connect.Option runtime.Chain returned) on a real httptest.Server,
// and returns a real Connect client pointed at it.
func newEchoServer(t *testing.T, chainOpt connect.Option) *connect.Client[wrapperspb.StringValue, wrapperspb.StringValue] {
	t.Helper()
	mux := http.NewServeMux()
	handler := connect.NewUnaryHandler(
		echoProcedure,
		func(_ context.Context, req *connect.Request[wrapperspb.StringValue]) (*connect.Response[wrapperspb.StringValue], error) {
			return connect.NewResponse(req.Msg), nil
		},
		chainOpt,
	)
	mux.Handle(echoProcedure, handler)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return connect.NewClient[wrapperspb.StringValue, wrapperspb.StringValue](ts.Client(), ts.URL+echoProcedure)
}

// TestGlobalValidator_PointerIdentityAcrossCalls is Test 5:
// protovalidate.GlobalValidator (the default runtime.Chain falls back to
// via connectrpc.com/validate's own NewInterceptor when no WithValidator
// override is given) is a fixed package-level value — reading it
// repeatedly always yields the identical Validator, never a fresh one.
func TestGlobalValidator_PointerIdentityAcrossCalls(t *testing.T) {
	first := protovalidate.GlobalValidator
	second := protovalidate.GlobalValidator
	if first != second {
		t.Fatalf("want protovalidate.GlobalValidator to be pointer-identical across reads, got distinct values %v, %v", first, second)
	}

	// Building runtime.Chain several times with no WithValidator override
	// must observe the identical default each time too — Chain never
	// substitutes a different validator when the caller supplies none.
	for i := 0; i < 4; i++ {
		if _, err := Chain(alwaysAllowAuthenticator{}); err != nil {
			t.Fatalf("Chain (default validator, iteration %d): %v", i, err)
		}
	}
	if protovalidate.GlobalValidator != first {
		t.Fatal("want protovalidate.GlobalValidator unchanged after building several default Chains")
	}
}

// TestChain_ValidatorBuiltOnceAcrossSequentialRequests is Test 6: a
// runtime.Chain built ONCE and driven through four SEQUENTIAL requests
// uses one validator instance — constructionCount stays at 1 for the
// entire test, and calls.Load() equals exactly the number of requests
// made, proving validation ran on every request without ever being
// reconstructed on the request path.
func TestChain_ValidatorBuiltOnceAcrossSequentialRequests(t *testing.T) {
	var calls atomic.Int64
	before := constructionCount.Load()
	v := newCountingValidator(&calls)

	chainOpt, err := Chain(alwaysAllowAuthenticator{}, WithValidator(v))
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	client := newEchoServer(t, chainOpt)

	const requests = 4
	for i := 0; i < requests; i++ {
		resp, err := client.CallUnary(context.Background(), connect.NewRequest(wrapperspb.String("hello")))
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if resp.Msg.GetValue() != "hello" {
			t.Fatalf("request %d: want echoed value %q, got %q", i, "hello", resp.Msg.GetValue())
		}
	}

	if got := constructionCount.Load(); got != before+1 {
		t.Fatalf("want exactly 1 validator construction across %d sequential requests (construction count delta), got delta %d", requests, got-before)
	}
	if got := calls.Load(); got != requests {
		t.Fatalf("want %d Validate calls (one per request), got %d", requests, got)
	}
}

// TestChain_ValidatorBuiltOnceUnderConcurrency is Test 7: at least eight
// goroutines concurrently BUILDING Chains (with the identical
// WithValidator-supplied instance, constructed exactly once before any
// goroutine starts) and ISSUING requests through a shared server still
// observe exactly one validator construction, and every concurrent
// Validate call is race-safe (go test -race is what actually proves the
// "safe under concurrency" half of this claim; the assertions below prove
// the "exactly one instance, exactly N calls" half).
func TestChain_ValidatorBuiltOnceUnderConcurrency(t *testing.T) {
	var calls atomic.Int64
	before := constructionCount.Load()
	v := newCountingValidator(&calls)

	const goroutines = 8

	// Phase 1: goroutines goroutines concurrently build a Chain from the
	// SAME v — proves Chain() itself performs no hidden construction, race
	// tested via go test -race.
	var buildWG sync.WaitGroup
	chainOpts := make([]connect.Option, goroutines)
	buildErrs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		buildWG.Add(1)
		go func(i int) {
			defer buildWG.Done()
			opt, err := Chain(alwaysAllowAuthenticator{}, WithValidator(v))
			chainOpts[i] = opt
			buildErrs[i] = err
		}(i)
	}
	buildWG.Wait()
	for i, err := range buildErrs {
		if err != nil {
			t.Fatalf("Chain (goroutine %d): %v", i, err)
		}
	}

	if got := constructionCount.Load(); got != before+1 {
		t.Fatalf("want exactly 1 validator construction across %d concurrent Chain builds (construction count delta), got delta %d", goroutines, got-before)
	}

	// Phase 2: one server mounted from one of the concurrently-built
	// Chains (they are all equivalent — same v), then goroutines
	// goroutines issue requests through it concurrently.
	client := newEchoServer(t, chainOpts[0])

	var reqWG sync.WaitGroup
	reqErrs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		reqWG.Add(1)
		go func(i int) {
			defer reqWG.Done()
			_, err := client.CallUnary(context.Background(), connect.NewRequest(wrapperspb.String("concurrent")))
			reqErrs[i] = err
		}(i)
	}
	reqWG.Wait()
	for i, err := range reqErrs {
		if err != nil {
			t.Errorf("request (goroutine %d): %v", i, err)
		}
	}

	if got := constructionCount.Load(); got != before+1 {
		t.Fatalf("want exactly 1 validator construction after %d concurrent requests too (construction count delta), got delta %d", goroutines, got-before)
	}
	if got := calls.Load(); got != goroutines {
		t.Fatalf("want %d Validate calls (one per concurrent request), got %d", goroutines, got)
	}
}
