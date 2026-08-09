package entc

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/smintz/entconnect/mixinforproto"
)

// ValidateMaskPaths is CRUD-05/D-17's build-time counterpart to
// runtime/fieldmask.go's request-time ValidateMask: it cross-checks
// every field on md (the Update binding's entity message descriptor,
// e.g. Patch) against sm (Phase 1's SourceMessage provenance for that
// same message) and allowed (the derived, ent-backed, non-excluded
// top-level field names entc/crud_update.go's generator computed),
// classifying each into exactly one of:
//
//   - satisfiable — present in sm.Fields, absent from sm.Excluded, and
//     present in allowed. No failure.
//   - nested or wildcard — the field's own name contains "." or equals
//     "*". Unreachable for a genuine protobuf field name (proto field
//     identifiers cannot contain either), kept for classification
//     completeness/documentation parity with runtime.ValidateMask's own
//     D-15 check.
//   - excluded — present in sm.Fields and in sm.Excluded: the schema
//     deliberately excluded this field from MixinForProto derivation.
//     ALWAYS a failure, with no exception for the entity's own
//     structural identifier ("id" — see below): if a field must never
//     be reachable as a mask path, the correct tool is keeping it off
//     the Update surface's message entirely, not Exclude()ing it and
//     hoping nobody asks.
//   - unknown — every other case: a field absent from sm.Fields
//     entirely, or present in sm.Fields and not excluded but still
//     missing from allowed (e.g. an Override()'d field, which installs
//     an ent.Field with no SourceField provenance — mixinforproto/
//     option.go's own documented behavior — or a message-typed field
//     the mixin skips by default). Both read the same way to a client
//     supplying that path: the field simply has no settable ent
//     counterpart.
//
// The entity's own reserved structural identifier field ("id" —
// mixinforproto/reserved.go's reservedStructural, always excluded by
// construction since every ent entity already has one) is never a mask-
// path candidate at all and is skipped entirely before classification —
// it is the lookup key, never an update target, and its exclusion is
// never itself a failure.
//
// Every finding is collected in one pass (D-05/D-09) — never returned on
// the first offense — and rendered in a deterministic sorted order
// (D-20/D-24).
func ValidateMaskPaths(md protoreflect.MessageDescriptor, sm mixinforproto.SourceMessage, allowed []string) []failure {
	msgName := string(md.FullName())

	fieldSet := make(map[string]bool, len(sm.Fields))
	for _, fr := range sm.Fields {
		fieldSet[fr.Name] = true
	}
	excludedSet := make(map[string]bool, len(sm.Excluded))
	for _, n := range sm.Excluded {
		excludedSet[n] = true
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, n := range allowed {
		allowedSet[n] = true
	}

	fds := md.Fields()
	var failures []failure
	satisfiable := 0
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		name := string(fd.Name())
		if name == "id" {
			continue
		}

		switch {
		case strings.Contains(name, ".") || name == "*":
			failures = append(failures, failure{
				schemaName: msgName, op: "update", sortKey: msgName,
				rule:        "mask-nested",
				description: fmt.Sprintf("candidate mask path %q on message %q is nested or a wildcard", name, msgName),
				remedy:      "only top-level paths are supported (D-15)",
			})
		case excludedSet[name]:
			failures = append(failures, failure{
				schemaName: msgName, op: "update", sortKey: msgName,
				rule:        "mask-excluded",
				description: fmt.Sprintf("field %q on message %q is deliberately excluded from MixinForProto derivation", name, msgName),
				remedy:      fmt.Sprintf("stop excluding %q if it must be reachable through the Update RPC, or remove it from the entity message entirely if it must never be", name),
			})
		case fieldSet[name] && allowedSet[name]:
			satisfiable++
		default:
			failures = append(failures, failure{
				schemaName: msgName, op: "update", sortKey: msgName,
				rule:        "mask-unknown",
				description: fmt.Sprintf("field %q on message %q has no corresponding settable ent field", name, msgName),
				remedy:      fmt.Sprintf("use Exclude(%q) to deliberately omit it, or check for an Override(%q, ...) that installs a field with no derivation provenance", name, name),
			})
		}
	}

	if satisfiable == 0 {
		failures = append(failures, failure{
			schemaName: msgName, op: "update", sortKey: msgName,
			rule:        "mask-zero-satisfiable",
			description: fmt.Sprintf("message %q has zero settable fields for its Update binding — every candidate field is excluded or unknown", msgName),
			remedy:      "derive at least one settable field, or remove the UpdateRPC binding entirely",
		})
	}

	sort.SliceStable(failures, func(i, j int) bool {
		if failures[i].rule != failures[j].rule {
			return failures[i].rule < failures[j].rule
		}
		return failures[i].description < failures[j].description
	})

	return failures
}
