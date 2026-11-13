# Operator console: test plan

Go (`internal/modules/operator/operator_test.go`, real Postgres, every module's tables):

| Case | Test |
| --- | --- |
| List: search (case-insensitive, wildcards literal, length limit), counts, source, connector state, last message | `TestTenantsAndDetail` |
| Detail: health counts, connector error, usage, setup steps, owner; unknown id; overview totals and plans | same |
| No permission → 403; permission without super admin → 403; deactivated super admin → 403 | `TestHandlers_OperatorsOnlyAndAudited` |
| No patient phone numbers or message text in the response | same |
| Audit: business's log gets `viewed_by_weline_staff` with the operator as actor; operator's log gets each view | same |

Isolation: `internal/modules/isolation_test.go` (every tenant module tries another organization; checked to fail for a
module without one); new cases in `reminders` (sends list, a reply event in another organization's name) and
`billing` (another organization's broadcasts don't count).

Frontend: `stores/operator.spec.ts`. Journey (`e2e/operator.e2e.ts`): the super admin opens the console, finds a
business, opens it and the view is audited; a business's own admin has no menu item and gets 403 from the API.
