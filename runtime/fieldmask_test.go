package runtime

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	entconnecttestv1 "github.com/smintz/entconnect/internal/gen/entconnecttestv1"
)

// allowedPatchFields mirrors the real fixture's derived, non-excluded
// field set (internal_note is deliberately absent — mirroring the real
// Patch schema's Exclude("internal_note")).
var allowedPatchFields = []string{"title", "body", "revision"}

func TestValidateMask(t *testing.T) {
	msg := &entconnecttestv1.Patch{}

	t.Run("nil mask", func(t *testing.T) {
		paths, err := ValidateMask(nil, msg, allowedPatchFields)
		if !errors.Is(err, ErrMaskEmpty) {
			t.Fatalf("want ErrMaskEmpty, got %v", err)
		}
		if paths != nil {
			t.Fatalf("want nil paths, got %v", paths)
		}
	})

	t.Run("zero-length paths", func(t *testing.T) {
		_, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: nil}, msg, allowedPatchFields)
		if !errors.Is(err, ErrMaskEmpty) {
			t.Fatalf("want ErrMaskEmpty, got %v", err)
		}
	})

	t.Run("empty-string path", func(t *testing.T) {
		_, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: []string{""}}, msg, allowedPatchFields)
		if !errors.Is(err, ErrMaskUnknown) {
			t.Fatalf("want ErrMaskUnknown, got %v", err)
		}
	})

	t.Run("dotted path", func(t *testing.T) {
		_, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: []string{"title", "patch.title"}}, msg, allowedPatchFields)
		if !errors.Is(err, ErrMaskNested) {
			t.Fatalf("want ErrMaskNested, got %v", err)
		}
		if err == nil || !strings.Contains(err.Error(), "patch.title") {
			t.Fatalf("want the offending path %q named in the error, got %v", "patch.title", err)
		}
	})

	t.Run("wildcard path", func(t *testing.T) {
		_, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: []string{"*"}}, msg, allowedPatchFields)
		if !errors.Is(err, ErrMaskNested) {
			t.Fatalf("want ErrMaskNested, got %v", err)
		}
	})

	t.Run("unknown path (not on descriptor)", func(t *testing.T) {
		_, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: []string{"does_not_exist"}}, msg, allowedPatchFields)
		if !errors.Is(err, ErrMaskUnknown) {
			t.Fatalf("want ErrMaskUnknown, got %v", err)
		}
	})

	t.Run("unknown path (on descriptor but excluded)", func(t *testing.T) {
		_, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: []string{"internal_note"}}, msg, allowedPatchFields)
		if !errors.Is(err, ErrMaskUnknown) {
			t.Fatalf("want ErrMaskUnknown, got %v", err)
		}
	})

	t.Run("duplicate path collapses to one", func(t *testing.T) {
		paths, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: []string{"title", "title"}}, msg, allowedPatchFields)
		if err != nil {
			t.Fatalf("want no error, got %v", err)
		}
		if want := []string{"title"}; !reflect.DeepEqual(paths, want) {
			t.Fatalf("want %v, got %v", want, paths)
		}
	})

	t.Run("case-differing path is rejected, never case-folded", func(t *testing.T) {
		_, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: []string{"Title"}}, msg, allowedPatchFields)
		if !errors.Is(err, ErrMaskUnknown) {
			t.Fatalf("want ErrMaskUnknown for a case-differing path, got %v", err)
		}
	})

	t.Run("reversed path order produces the same sorted output", func(t *testing.T) {
		forward, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: []string{"body", "revision", "title"}}, msg, allowedPatchFields)
		if err != nil {
			t.Fatalf("forward: want no error, got %v", err)
		}
		reversed, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: []string{"title", "revision", "body"}}, msg, allowedPatchFields)
		if err != nil {
			t.Fatalf("reversed: want no error, got %v", err)
		}
		if !reflect.DeepEqual(forward, reversed) {
			t.Fatalf("want order-independent output: forward=%v reversed=%v", forward, reversed)
		}
		want := []string{"body", "revision", "title"}
		if !reflect.DeepEqual(forward, want) {
			t.Fatalf("want sorted %v, got %v", want, forward)
		}
	})

	t.Run("valid single path", func(t *testing.T) {
		paths, err := ValidateMask(&fieldmaskpb.FieldMask{Paths: []string{"title"}}, msg, allowedPatchFields)
		if err != nil {
			t.Fatalf("want no error, got %v", err)
		}
		if want := []string{"title"}; !reflect.DeepEqual(paths, want) {
			t.Fatalf("want %v, got %v", want, paths)
		}
	})
}
