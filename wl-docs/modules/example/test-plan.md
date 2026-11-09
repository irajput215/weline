# Example: test plan

| Case | Level | Where |
| --- | --- | --- |
| Create trims text, lists newest first | service, real Postgres | `internal/modules/example/example_test.go` `TestService_CreateAndList` |
| Empty, whitespace and over-long (by characters) text rejected | service | `TestService_RejectsBadText` |
| Org B can't see org A's notes | service | `TestService_OrganizationsAreIsolated` |
| Create then list through the real route wiring | handler, real Postgres | `TestHandlers_CreateThenList` |
| Second organization sees nothing over the API | handler | `TestHandlers_SecondOrganizationCannotSeeNotes` |
| 403 without read, 403 create when read-only, 400 empty text, 401 not logged in | handler | `TestHandlers_Refusals` |
| Module builds with nil app; permissions valid | unit | `TestModule_ContractWithoutApp` |
| Migration down then up | real Postgres | `TestMigrations_AreReversible` |
| Store: load, forbidden on 403, error otherwise, add prepends, refused save | Vitest | `frontend/src/modules/example/stores/notes.spec.ts` |
| Route and nav carry `wl_example_` permissions | Vitest | `frontend/src/modules/index.spec.ts` |
| Admin adds a note; agent without permission is kept out; read-only user sees the forbidden message; second org sees an empty list | manual browser run on a production build | step 0.7 PR |
