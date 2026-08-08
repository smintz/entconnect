---
schema_version: 1
open_count: 4
waived_count: 0
fixed_count: 0
total_count: 4
last_updated: 2026-08-08T13:38:47.295Z
---

# Broken Windows Ledger

> Cross-phase defect register. With `workflow.windows_enforce` enabled, `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 01 | deviation | mixinforproto/fieldmap.go |  | Unsigned integer interval constraints (uint32/uint64/fixed32/fixed64 gt/gte/lt/lte) are recorded as residual, not translated to Min/Max/Range — deliberate scope boundary (01-05-PLAN.md Task 3 scoped signed-int/float only); never silently dropped, but not yet exact for unsigned bounds | open |  | 2026-08-08T11:57:08.681Z |  |
| 2 | 01 | deviation | mixinforproto/fieldmap.go |  | bytes.min_len/max_len/len/pattern Tier 1 translation is out of scope this plan (VAL-01 names string specifically); buildBytesField only translates required (NotEmpty); a real bytes.* length/pattern constraint would currently derive with no builder call and no residual record | open |  | 2026-08-08T11:57:08.825Z |  |
| 3 | 01 | deviation | mixinforproto/fieldmap.go |  | Repeated scalar and repeated enum fields fail loudly at schema load rather than mapping to a list-typed ent field; this is a deliberate v0.1 boundary consistent with D-10's no-silent-approximation posture, not a silent gap (closes 01-VERIFICATION.md gap 1 / CR-01). A real list-typed ent mapping is the follow-up. | open |  | 2026-08-08T13:28:07.454Z |  |
| 4 | 01 | deviation | proto/mixinforprototest.binpb |  | Descriptor set (proto/mixinforprototest.binpb) was not regenerated when 01-06 added repeated.proto to the corpus; running scripts/pipeline.sh's step 3 against the current tree produces a differing binpb. Discovered during 01-07 verification; out of scope to fix here (this plan changes scripts/Makefile/CI only, not generated artifacts) — restored to committed state after detection. | open |  | 2026-08-08T13:38:47.295Z |  |

````json
[
  {
    "id": 1,
    "kind": "deviation",
    "phase": "01",
    "file": "mixinforproto/fieldmap.go",
    "line": null,
    "description": "Unsigned integer interval constraints (uint32/uint64/fixed32/fixed64 gt/gte/lt/lte) are recorded as residual, not translated to Min/Max/Range — deliberate scope boundary (01-05-PLAN.md Task 3 scoped signed-int/float only); never silently dropped, but not yet exact for unsigned bounds",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-08T11:57:08.681Z",
    "resolved_at": null
  },
  {
    "id": 2,
    "kind": "deviation",
    "phase": "01",
    "file": "mixinforproto/fieldmap.go",
    "line": null,
    "description": "bytes.min_len/max_len/len/pattern Tier 1 translation is out of scope this plan (VAL-01 names string specifically); buildBytesField only translates required (NotEmpty); a real bytes.* length/pattern constraint would currently derive with no builder call and no residual record",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-08T11:57:08.825Z",
    "resolved_at": null
  },
  {
    "id": 3,
    "kind": "deviation",
    "phase": "01",
    "file": "mixinforproto/fieldmap.go",
    "line": null,
    "description": "Repeated scalar and repeated enum fields fail loudly at schema load rather than mapping to a list-typed ent field; this is a deliberate v0.1 boundary consistent with D-10's no-silent-approximation posture, not a silent gap (closes 01-VERIFICATION.md gap 1 / CR-01). A real list-typed ent mapping is the follow-up.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-08T13:28:07.454Z",
    "resolved_at": null
  },
  {
    "id": 4,
    "kind": "deviation",
    "phase": "01",
    "file": "proto/mixinforprototest.binpb",
    "line": null,
    "description": "Descriptor set (proto/mixinforprototest.binpb) was not regenerated when 01-06 added repeated.proto to the corpus; running scripts/pipeline.sh's step 3 against the current tree produces a differing binpb. Discovered during 01-07 verification; out of scope to fix here (this plan changes scripts/Makefile/CI only, not generated artifacts) — restored to committed state after detection.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-08T13:38:47.295Z",
    "resolved_at": null
  }
]
````
