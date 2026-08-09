package runtime

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ErrMaskEmpty is returned (D-16) when a mask is nil or carries zero
// paths. An empty or absent mask is NEVER treated as "update
// everything" — that is the exact proto3 zero-collapse footgun
// mixinforproto/README.md's presence section documents: a Set* for
// every field on a proto message silently clears every field the caller
// left at its Go zero value.
var ErrMaskEmpty = errors.New("entconnect: update_mask is required and must not be empty")

// ErrMaskNested is returned (D-15) when a mask path contains "." or is
// the wildcard "*". fieldmaskpb.FieldMask.IsValid ACCEPTS nested paths
// by design (it recurses into the nested message descriptor) — this
// project's top-level-only rule is an entconnect-specific restriction
// layered on top, not something fieldmaskpb enforces on its own.
var ErrMaskNested = errors.New("entconnect: update_mask path is nested or a wildcard — only top-level paths are supported")

// ErrMaskUnknown is returned when a mask path is not valid against
// msg's descriptor (google.protobuf.FieldMask.IsValid), or is valid
// against the descriptor but is not a member of the caller-supplied
// allowed set (a field the schema excluded from derivation, or one
// with no corresponding settable ent field).
var ErrMaskUnknown = errors.New("entconnect: update_mask path names no settable field")

// ValidateMask validates mask against msg's live descriptor and allowed
// (the derived, non-excluded, ent-backed top-level field names the
// generated Update handler computed at codegen time — D-17), per
// D-14..D-16. Checks run in this fixed order, every one of which must
// pass before the next runs:
//
//  1. a nil mask, or a mask whose GetPaths() is empty, returns
//     ErrMaskEmpty (D-16) — never "update everything".
//  2. any path containing "." or equal to the wildcard "*" returns an
//     error wrapping ErrMaskNested, naming the offending path (D-15).
//     This check is explicit and independent of fieldmaskpb.IsValid,
//     which accepts nested paths as a designed feature.
//  3. mask.IsValid(msg) for descriptor validity, and additionally a
//     membership check of every path against allowed, returns an error
//     wrapping ErrMaskUnknown naming the offending path.
//  4. on success, the deduplicated paths are returned in sorted order —
//     duplicate paths collapse to one, and path order in the request
//     never changes the resulting row.
//
// Path matching is exact byte equality throughout — no case folding, no
// Unicode normalization: a path differing from a real field name only
// by letter case is rejected as unknown, not silently accepted.
func ValidateMask(mask *fieldmaskpb.FieldMask, msg proto.Message, allowed []string) ([]string, error) {
	if mask == nil || len(mask.GetPaths()) == 0 {
		return nil, ErrMaskEmpty
	}

	for _, p := range mask.GetPaths() {
		if strings.Contains(p, ".") || p == "*" {
			return nil, fmt.Errorf("entconnect: update_mask path %q is nested or a wildcard: %w", p, ErrMaskNested)
		}
	}

	// Checked per-path (not via a single mask.IsValid(msg) call over the
	// whole slice) so a descriptor-unknown path can be named verbatim in
	// the error — IsValid over the full slice only reports whether ALL
	// paths are valid, not which one failed.
	for _, p := range mask.GetPaths() {
		if !(&fieldmaskpb.FieldMask{Paths: []string{p}}).IsValid(msg) {
			return nil, fmt.Errorf(
				"entconnect: update_mask path %q is not present on the message descriptor %q: %w",
				p, msg.ProtoReflect().Descriptor().FullName(), ErrMaskUnknown,
			)
		}
	}

	allowedSet := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = true
	}

	dedup := make(map[string]bool, len(mask.GetPaths()))
	for _, p := range mask.GetPaths() {
		if !allowedSet[p] {
			return nil, fmt.Errorf("entconnect: update_mask path %q is not a settable field: %w", p, ErrMaskUnknown)
		}
		dedup[p] = true
	}

	paths := make([]string, 0, len(dedup))
	for p := range dedup {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}
