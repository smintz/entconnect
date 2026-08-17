package runtime

import (
	"errors"
	"fmt"
	"testing"

	"buf.build/go/protovalidate"
	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"connectrpc.com/connect"
	"entgo.io/ent/privacy"
)

// TestMapErrorValidationError asserts Task 3's new case: a
// *protovalidate.ValidationError maps to CodeInvalidArgument — the same
// mapping connectrpc.com/validate's own boundary interceptor produces
// for the identical Go type, which is what makes VAL-07's identity
// guarantee concrete in code.
func TestMapErrorValidationError(t *testing.T) {
	ve := &protovalidate.ValidationError{
		Violations: []*protovalidate.Violation{
			{Proto: &validate.Violation{
				RuleId:  strPtr("constraints.residual_cel.starts_with_x"),
				Message: strPtr("value must start with X"),
			}},
		},
	}

	err := MapError(ve)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument, got %v (err: %v)", connect.CodeOf(err), err)
	}

	// The returned *connect.Error must carry ve as its underlying error,
	// so an adopter can errors.As back to it (VAL-06's "consumable
	// outside any RPC context" promise, made concrete at the wire-error
	// boundary too).
	var got *protovalidate.ValidationError
	if !errors.As(err, &got) {
		t.Fatalf("want errors.As(err, &got) to find the original *protovalidate.ValidationError, got err=%v", err)
	}
	if got != ve {
		t.Fatalf("want the exact same *protovalidate.ValidationError instance wrapped, got a different one")
	}
}

// TestMapErrorValidationErrorWrapped asserts the new case uses
// errors.As, not a bare type assertion: a *protovalidate.ValidationError
// wrapped by an intermediate fmt.Errorf("%w", ...) must still map to
// CodeInvalidArgument.
func TestMapErrorValidationErrorWrapped(t *testing.T) {
	ve := &protovalidate.ValidationError{
		Violations: []*protovalidate.Violation{
			{Proto: &validate.Violation{RuleId: strPtr("string.min_len")}},
		},
	}
	wrapped := fmt.Errorf("mutation rejected: %w", ve)

	err := MapError(wrapped)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("want CodeInvalidArgument for a wrapped ValidationError, got %v (err: %v)", connect.CodeOf(err), err)
	}
}

// TestMapErrorPrivacyDenyStillWins asserts the new ValidationError case
// is not ordered ahead of the existing errors.Is(err, privacy.Deny)
// branch — privacy.Deny must still map to CodePermissionDenied.
func TestMapErrorPrivacyDenyStillWins(t *testing.T) {
	err := MapError(privacy.Deny)
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("want CodePermissionDenied for privacy.Deny, got %v (err: %v)", connect.CodeOf(err), err)
	}
}

// TestMapErrorConnectErrorPassthrough asserts an already-typed
// *connect.Error still passes through unchanged.
func TestMapErrorConnectErrorPassthrough(t *testing.T) {
	original := connect.NewError(connect.CodeUnauthenticated, errors.New("no viewer"))
	err := MapError(original)
	if err != original {
		t.Fatalf("want the exact same *connect.Error returned unchanged, got a different error: %v", err)
	}
}

// TestMapErrorUnrecognisedIsInternal asserts an error MapError cannot
// classify still maps to CodeInternal with a generic message (never the
// raw internal error text on the wire).
func TestMapErrorUnrecognisedIsInternal(t *testing.T) {
	err := MapError(errors.New("boom: something exploded internally"))
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("want CodeInternal, got %v (err: %v)", connect.CodeOf(err), err)
	}
	if got := err.Error(); got == "boom: something exploded internally" {
		t.Fatalf("want a generic message, not the raw internal error text verbatim: %q", got)
	}
}

// strPtr is a tiny local helper so this test file does not need to
// import google.golang.org/protobuf/proto solely for one-line pointer
// construction across several test cases.
func strPtr(s string) *string { return &s }
