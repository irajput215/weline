# Onboarding: test plan

Go (`internal/modules/onboarding/onboarding_test.go`, real Postgres):

| Case | Test |
| --- | --- |
| Sign-up creates organization, slug, admin user and membership, practice profile and time zone, audit entry | `TestSignup_CreatesATenantWithModulePermissions` |
| The new organization's manager gets module permissions | same |
| Same email (any letter case) refused | same |
| Each invalid field refused; nothing created when refused inside the transaction | `TestSignup_Validation` |
| Checklist steps tick off from data; another organization's data doesn't count; vertical validated; hide/show | `TestChecklist` |
| Every route refuses without permission; read can't update; vertical and dismissed through the API | `TestHandlers` |
| Public sign-up closed by default, refused without robot-check keys, failed check, success, rate limit, page and CSP, closed page | `TestPublicSignup` |
| Source link to the running commit, override | `TestSourceURL` |

Kit: `TestGrantMissing_NewOrganizationsGetModulePermissionsOnce` (named roles only, custom roles untouched, cache
callback, a removed grant stays removed, organizations without roles wait).

Frontend: `stores/onboarding.spec.ts`. Journeys (`e2e/onboarding.e2e.ts`): sign up on the public page, sign in, the
checklist shows business done, choose the type, open the connectors page (CI, needs Turnstile test keys and
`WELINE_SIGNUP=open`); an existing admin hides and shows the checklist; reception has none; `/x/source` redirects.

Gate (manual): a new business signs up on staging, connects WhatsApp and Cliniko (or built-in) and sends its first
message without our help.
