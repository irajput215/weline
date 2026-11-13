# Reminders and recalls: test plan

Go (`internal/modules/reminders/reminders_test.go`, real Postgres, fake clock, in-memory connector and sender):

| Case | Test |
| --- | --- |
| Rule validation: offset/recall ranges, kind, unapproved template, language, recall variables | `TestRules` |
| Another organization can't change or delete a rule | `TestRules` |
| **Due now** sends with the right variables; later not yet | `TestSweep_DueAlreadyCancelledMoved` |
| **Already sent**: a second sweep sends nothing | same |
| **Cancelled** appointments are never reminded | same |
| **Moved**: nothing for the old time, one for the new time when due | same |
| Contact created from the connector's mobile | same |
| Opted-out (service messages stopped after STOP) and no mobile are skipped with reasons | `TestSweep_ConsentAndNoMobile` |
| Confirm / Cancel / Reschedule applied through the connector or a handover, replied to, and **claimed** | `TestReplies` |
| An answered reminder or an unrelated message isn't a reply | `TestReplies` |
| Recall due list excludes recent and already-booked customers | `TestRecalls` |
| Recall send skips marketing opt-outs and the not-due; once per month | `TestRecalls` |
| Every route refuses without its permission; isolation between organizations; reminder rule isn't a recall | `TestHandlers` |

Elsewhere: `kit/events_claim_test.go` (claims), `ai/agent_test.go` (claimed messages are left alone),
`booking/provider_test.go` and `cliniko/cliniko_test.go` (`GetCustomer`).

Frontend: `stores/reminders.spec.ts` (states, forbidden, rule CRUD, server messages, recall send). Journey
`e2e/reminders.e2e.ts`: a manager adds a reminder and a recall rule from an approved template, sees a send with its
reply and an empty due list; reception sees rules and activity but can't add rules or open recalls.

Gate (manual, staging with the Cliniko trial): several days of reminders with no duplicates or misses; Confirm,
Reschedule and Cancel from a real phone; a recall run reaches only due, consenting patients.
