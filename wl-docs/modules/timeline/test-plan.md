# Contact timeline: test plan

Go (`internal/modules/timeline/timeline_test.go`, real Postgres, every source seeded):

| Case | Test |
| --- | --- |
| Every source appears, newest first, upcoming appointment on top; fields; customer found, never created; no message text, notes, evidence or full number; another contact's items absent; another organization gets not found; connected system down → the rest with a note | `TestTimeline_AllSourcesNoTextAndIsolated` |
| 100 a page, cursor continues with the rest | `TestTimeline_Paging` |
| Search by name and by part of the number, masked, wildcards literal, too short refused, another organization finds nobody; mask format | `TestSearch` |
| Routes refuse without permission; bad id 404; bad cursor and short search 400 | `TestHandlers` |

Cliniko: `TestFindCustomer_NeverCreates`. Frontend: `stores/timeline.spec.ts`. Journey (`e2e/timeline.e2e.ts`):
reception finds a contact, sees a consent item without the evidence or the full number; an unknown contact says so.
