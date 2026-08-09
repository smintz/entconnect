---
phase: 02-crud-handlers-interceptor-chain
plan: 03
subsystem: api
tags: [entc, connect-rpc, protobuf, ent, keyset-pagination, aip-158, codegen]

# Dependency graph
requires:
  - phase: 02-crud-handlers-interceptor-chain
    provides: "02-01's entc.Extension skeleton (RegisterGenerator registry, gen.Hook-based emission, goImportPath/connectImportPath descriptor helpers, decodeAnnotation[T] JSON round-trip, the failure/generateError discipline) and runtime/ framework (Authenticator, Chain, Server/Route, MapError, viewer.Viewer)"
provides:
  - "A corrected CRUD-03/ROADMAP Success Criterion 2, retiring the disproven native-ent-Paginate() claim with dated evidence pointing at 02-RESEARCH.md Q1"
  - "runtime/cursor.go — the opaque page_token codec: Fingerprint (length-prefixed SHA-256), EncodeCursor, DecodeCursor, ErrCursorMismatch, ErrCursorMalformed, with the unsigned-cursor decision documented in source"
  - "entc/crud_list.go + entc/templates/list.tmpl — the OpList generator, registered via init() with no edit to entc/crud.go, emitting hand-built keyset Where/Order/Limit calls over ent's real per-field comparison operators"
  - "internal/entconnecttest/list/ — a real, generated, tested fixture app proving the List slice end to end: sequential pages concatenating to the full ordered set, ties at a page boundary, empty/boundary result sets, ordering stability, and fingerprint/malformed-token rejection"
affects: [02-04-update-fieldmask, 02-05-manual-drift-report, phase-3-validation, phase-5-drift-check]

# Actuals (#2632)
actuals:
  tokens: 42000
  tasks: 3
  commits: 3

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Keyset list paging emitted by hand against ent's own generated predicate/order package (no entgql, no Paginate()): per-field GT/EQ predicates, And/Or combinators, Order(...OrderOption), Limit(pageSize+1)"
    - "Ordering tuple built from Binding.OrderBy (mapped through each field's mixinforproto.SourceField.FieldName provenance) with the entity's primary key always appended last, making the tuple total and unique by construction"
    - "Opaque page_token = base64(JSON{fingerprint, keys}) with a length-prefixed SHA-256 fingerprint over the ordering field names, deliberately unsigned for this phase (documented tradeoff, not an oversight)"
    - "entc/ generated code references a per-entity ent subpackage (e.g. `page`) by its bare local identifier only, relying on golang.org/x/tools/imports.Process to resolve the import automatically — same mechanism already proven for the sibling `ent` package in 02-01"

key-files:
  created:
    - runtime/cursor.go
    - runtime/cursor_test.go
    - entc/crud_list.go
    - entc/templates/list.tmpl
    - proto/entconnecttest/v1/list.proto
    - internal/entconnecttest/list/ent/schema/page.go
    - internal/entconnecttest/list/entconnect/page_list_service.entconnect.go
    - internal/entconnecttest/list/list_test.go
    - internal/entconnecttest/list/paging_edges_test.go
  modified:
    - .planning/REQUIREMENTS.md
    - .planning/ROADMAP.md
    - proto/descriptorset.binpb

key-decisions:
  - "CRUD-03 and ROADMAP Success Criterion 2 corrected in place (scoped edits, not rewrites) to describe hand-emitted keyset predicates over ent's own comparison operators, each carrying a 2026-08-08 correction note naming 02-RESEARCH.md Q1 as evidence"
  - "The page_token cursor is deliberately left unsigned for this phase (RESEARCH.md Assumption A1): AIP-158 requires only opacity, and ent's privacy Filter rules still apply to every paged query, so a forged cursor cannot bypass authorization — residual exposure (narrow ID-existence probing) and the HMAC-signing hardening option are both recorded in runtime/cursor.go's doc comment, not left as an unexamined default"
  - "listFieldSpec carries its own GoType field (rather than reverse-parsing it from a rendered expression) so decodeStmtFor and newListFieldSpec share one source of truth for a field's string<->native-type conversion shape"
  - "page_size clamp bounds (default 50, max 100) are computed once in entc/crud_list.go's Go constants and threaded into the template as data, so the emitted named constants and the generator's own bounds can never drift apart"

patterns-established:
  - "Pattern: a generator that needs to reference a per-entity ent subpackage (order/predicate helpers, not just the ent.Client type) writes the bare package identifier (req.Type.Package()) and trusts imports.Process to resolve the import — verified working end to end via the generated internal/entconnecttest/list/entconnect/page_list_service.entconnect.go file's own correctly-resolved `page` import"

requirements-completed: [CRUD-03]

coverage:
  - id: D1
    description: "CRUD-03 in REQUIREMENTS.md and ROADMAP.md Success Criterion 2 no longer attribute keyset paging to a native ent helper, and each carries a pointer to the evidence"
    requirement: "CRUD-03"
    verification:
      - kind: other
        ref: "grep -q 'hand-emitted keyset predicates' .planning/REQUIREMENTS.md .planning/ROADMAP.md && ! grep -rq 'Pagin' .planning/REQUIREMENTS.md .planning/ROADMAP.md"
        status: pass
    human_judgment: false
  - id: D2
    description: "A schema annotates entconnect.ListRPC(<Procedure>, <order fields>) and a generated Connect List handler returns a first page plus an AIP-158 next_page_token"
    requirement: "CRUD-03"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/list/list_test.go#TestListPages_FirstPageShape"
        status: pass
    human_judgment: false
  - id: D3
    description: "Passing the returned next_page_token to a second List call continues from exactly where the first page stopped — no row skipped, no row repeated"
    requirement: "CRUD-03"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/list/list_test.go#TestListPages_SequentialPagesConcatenateToFullSet"
        status: pass
    human_judgment: false
  - id: D4
    description: "Paging is expressed as keyset predicates plus a limit of pageSize+1 for next-page detection; no generated query uses an offset"
    requirement: "CRUD-03"
    verification:
      - kind: other
        ref: "grep -rniE 'offset' internal/entconnecttest/list/entconnect entc/crud_list.go entc/templates/list.tmpl — no matches"
        status: pass
    human_judgment: false
  - id: D5
    description: "Two rows whose ordering-field values are exactly equal are separated deterministically by the primary key appended to the ordering tuple, so a page boundary landing between them loses and duplicates nothing"
    requirement: "CRUD-03"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/list/paging_edges_test.go#TestPagingEdges_TiesAtBoundary"
        status: pass
    human_judgment: false
  - id: D6
    description: "An empty table returns an empty page and no next_page_token; a table with exactly pageSize rows returns one page and no next_page_token; a table with pageSize+1 rows returns a token"
    requirement: "CRUD-03"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/list/paging_edges_test.go#TestPagingEdges_Empty,TestPagingEdges_ExactlyPageSizeRows,TestPagingEdges_PageSizePlusOneRows"
        status: pass
    human_judgment: false
  - id: D7
    description: "A page_token whose embedded fingerprint disagrees with the current request's ordering fields fails with CodeInvalidArgument rather than returning a silently inconsistent page"
    requirement: "CRUD-03"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/list/paging_edges_test.go#TestPagingEdges_FingerprintMismatch,TestPagingEdges_MalformedToken"
        status: pass
    human_judgment: false
  - id: D8
    description: "Output order is total and stable across repeated calls with identical inputs, including when ordering-field values tie"
    requirement: "CRUD-03"
    verification:
      - kind: e2e
        ref: "internal/entconnecttest/list/paging_edges_test.go#TestPagingEdges_OrderingStability"
        status: pass
    human_judgment: false

duration: 15min
completed: 2026-08-09
status: complete
---

# Phase 2 Plan 3: List Handler & Keyset Paging Summary

**Hand-emitted keyset List paging over ent's own comparison operators (no `Paginate()`, no entgql dependency), with a fingerprinted unsigned page_token, plus the CRUD-03 requirement-text correction that makes the requirement honestly verifiable.**

## Performance

- **Duration:** ~15 min
- **Started:** 2026-08-09T00:59:56Z (approx, first task commit)
- **Completed:** 2026-08-09T01:14:45Z
- **Tasks:** 3 (all `type="auto"`)
- **Files modified:** 34 (3 hand-edited docs/binary + 6 hand-written Go/proto/template files + 25 ent-generated fixture files)

## Accomplishments

- CRUD-03 in `.planning/REQUIREMENTS.md` and ROADMAP.md's Phase 2 Success Criterion 2 are corrected in place, each carrying a dated (2026-08-08) correction note naming `02-RESEARCH.md` §Summary/Pitfall 1 as the evidence that retired the original "native ent `Paginate()`" claim.
- `runtime/cursor.go` ships the opaque `page_token` codec: `Fingerprint` (length-prefixed SHA-256, collision-resistant across part boundaries), `EncodeCursor`/`DecodeCursor`, and the `ErrCursorMismatch`/`ErrCursorMalformed` sentinels — with the deliberate unsigned-cursor tradeoff documented directly in the `Cursor` type's doc comment, naming the residual exposure and the deferred HMAC-signing option.
- `entc/crud_list.go` + `entc/templates/list.tmpl` implement the `OpList` generator (registered via `init()`, zero edits to `entc/crud.go`): it builds the ordering tuple from `Binding.OrderBy` mapped through each field's `mixinforproto.SourceField` provenance, always appends the entity's primary key last, and renders a `Limit(pageSize+1)` probe, an all-ascending `Order(...)` call, and — when a `page_token` is present — the standard lexicographic keyset disjunction (`Or`/`And` over `GT`/`EQ` predicates) with fingerprint verification mapped to `CodeInvalidArgument`.
- A complete, generated, tested fixture app (`internal/entconnecttest/list/`) proves the whole path end to end: sequential pages over 7 seeded rows concatenate to the full ordered set with every row exactly once; a page boundary falling directly between two byte-identical `created_at` values (at two different page sizes) loses and duplicates nothing; empty/single/exactly-full/one-over result sets each produce the correct `next_page_token` presence; five repeated identical calls return a byte-identical id sequence; a fingerprint-mismatched or malformed token fails `CodeInvalidArgument`; and `page_size` clamping is pinned to exactly the documented default (50) and max (100) bounds — all under `-race -count=5`.

## Task Commits

Each task was committed atomically:

1. **Task 1: Retire CRUD-03's disproven paging claim, with evidence** — `2d7f569` (docs)
2. **Task 2: List end to end — first page, next_page_token, second page continues exactly** — `e5a8d6f` (feat)
3. **Task 3: Paging edges — ties, empty and boundary result sets, and fingerprint mismatch** — `ad9e6ef` (test)

**Plan metadata:** *(this commit, docs: complete plan)*

## Files Created/Modified

- `.planning/REQUIREMENTS.md`, `.planning/ROADMAP.md` — CRUD-03 / Success Criterion 2 corrected, each with a dated evidence note
- `runtime/cursor.go`, `runtime/cursor_test.go` — the D-08 page_token codec and its unit tests
- `entc/crud_list.go`, `entc/templates/list.tmpl` — the `OpList` generator and its rendered method template
- `proto/entconnecttest/v1/list.proto`, `internal/gen/entconnecttestv1/list.pb.go`, `internal/gen/entconnecttestv1/entconnecttestv1connect/list.connect.go` — the `PageListService` fixture contract and its generated stubs
- `proto/descriptorset.binpb` — regenerated to include the new corpus file
- `internal/entconnecttest/list/ent/schema/page.go`, `ent/entc.go`, `ent/generate.go`, plus the full generated `ent/` package and `entconnect/page_list_service.entconnect.go`
- `internal/entconnecttest/list/list_test.go`, `paging_edges_test.go` — the round-trip proof and the probe-derived edge cases

## Decisions Made

See `key-decisions` in frontmatter above.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `entc/crud.go`'s `RegisterGenerator` registry now carries `OpGet` (02-01) and `OpList` (this plan) with zero edits to the registry file itself, confirming the registration mechanism scales cleanly to 02-04's `OpUpdate`.
- `runtime/cursor.go`'s `Fingerprint`/`EncodeCursor`/`DecodeCursor` are generic enough to be reused as-is if a later phase needs an opaque-token mechanism elsewhere; no changes anticipated.
- The imports.Process-resolves-the-per-entity-subpackage pattern (this plan's one new codegen technique beyond 02-01) is now proven and available to 02-04's Update generator if it needs to reference entity-specific predicate helpers.
- Nothing outstanding or blocking for 02-04/02-05.

---
*Phase: 02-crud-handlers-interceptor-chain*
*Completed: 2026-08-09*

## Self-Check: PASSED

All key created files verified present on disk (`runtime/cursor.go`, `entc/crud_list.go`,
`internal/entconnecttest/list/paging_edges_test.go`); all 3 task commit hashes (`2d7f569`,
`e5a8d6f`, `ad9e6ef`) verified present in `git log --oneline --all`. No missing items.
