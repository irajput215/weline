# Operator console: requirements

**For:** Weline staff running the shared service (ADR-005): who signed up, which plan, how much they use, whether
their WhatsApp and connected system work. Build guide step 6.3
([`../../build-guide/phase-6-saas.md`](../../build-guide/phase-6-saas.md)).

Status: **built.**

## User stories

- As a Weline operator, I see totals (businesses, sign-ups this week and month, plans, systems in error, businesses
  with failed sends) and a list of every business I can search.
- As an operator helping a business, I open it and see its plan, setup progress, this month's usage, a week of
  problems (failed messages, failed or skipped reminders, AI errors, its system's last error) and who signed it up.
- As a business admin, I can see in my audit log when Weline staff looked at my business.

## Functional requirements

- R1 **Operators only:** upstream **super admins** (read from the database on every request, active users only),
  checked on top of `wl_operator_console`, a permission no role is given. Granting it to a custom role alone opens
  nothing.
- R2 **Overview:** businesses, self sign-ups in 7 and 30 days, businesses per plan (and not billed), connected systems
  in error, businesses with messages WhatsApp refused in 7 days.
- R3 **Businesses list:** name, joined, how it started (self sign-up or created by staff), plan and status, team and
  contact counts, WhatsApp numbers, connected system and status, last message time; search by name (literal match),
  50 a page.
- R4 **One business:** R3 plus owner (name and email of who signed up), setup steps done (of 6), Stripe customer yes/no,
  this month's broadcast recipients and AI replies, and 7 days of failed messages, failed and skipped reminders, AI
  model errors, the system's last error and last sync.
- R5 **No patient data:** counts and states only; never contacts' names, numbers or message text.
- R6 **Audited:** every overview, list and business view is recorded in the operator's organization; viewing a
  business is also recorded in **that business's** audit log as "viewed by Weline staff", with the operator's name.
- R7 **Isolation review:** every tenant module has a test that tries another organization; a guard test fails when a
  module doesn't (`internal/modules/isolation_test.go`). Findings in
  [`../../tenant-isolation-review.md`](../../tenant-isolation-review.md).

## Non-functional

- Read-only; no tables of its own.
- The list's counts are per-business subqueries: fine for hundreds of businesses; add summary tables before thousands.
