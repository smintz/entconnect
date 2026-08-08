---
schema_version: 1
open_count: 2
waived_count: 0
fixed_count: 0
total_count: 2
last_updated: 2026-08-08T11:57:08.825Z
---

# Broken Windows Ledger

> Cross-phase defect register. With `workflow.windows_enforce` enabled, `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 01 | deviation | mixinforproto/fieldmap.go |  | Unsigned integer interval constraints (uint32/uint64/fixed32/fixed64 gt/gte/lt/lte) are recorded as residual, not translated to Min/Max/Range — deliberate scope boundary (01-05-PLAN.md Task 3 scoped signed-int/float only); never silently dropped, but not yet exact for unsigned bounds | open |  | 2026-08-08T11:57:08.681Z |  |
| 2 | 01 | deviation | mixinforproto/fieldmap.go |  | bytes.min_len/max_len/len/pattern Tier 1 translation is out of scope this plan (VAL-01 names string specifically); buildBytesField only translates required (NotEmpty); a real bytes.* length/pattern constraint would currently derive with no builder call and no residual record | open |  | 2026-08-08T11:57:08.825Z |  |

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
  }
]
````
