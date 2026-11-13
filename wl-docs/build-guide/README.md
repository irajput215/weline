# Weline Build Guide: Step-by-Step Prompts for Claude Code

## Session rules

- Every new session starts by reading `CLAUDE.md` and `wl-docs/build-guide/README.md`.
- I will name one step (for example "do step 0.4"). Do only that step, on its own branch from `dev`,
  then stop and show me the PR. Never start the next step on your own.
- Never add or change files under `docs/` (that is whatomate's website). Our docs go in `wl-docs/`.
- Before writing tests, check that Postgres and Redis are available. If not, tell me.
- If a step needs something you can't do here (GitHub settings, Meta tokens, a real WhatsApp number,
  a public webhook URL), stop and tell me exactly what I need to do.
- When a step is done, tick it in the progress tracker in `README.md` in the same PR.

---

This guide turns the Weline Module SDLC doc into an ordered list of prompts you paste into Claude Code, one at a time. Each step is one branch and one pull request into `dev`.

**Direction ([ADR-002](../adr/ADR-002-connect-dont-build-pms.md), 2026-11-12):** Weline connects WhatsApp to the system each business already uses (Cliniko first) instead of building a practice management system. Phases 2 to 7 below follow that order.

## How to use this guide

1. Start every step on a fresh branch from `dev` (the prompt tells Claude to do it).
2. Paste the prompt. Let Claude plan first, read the plan, then say "go".
3. Check the Done when list yourself before merging. Don't trust "it works": run it.
4. Merge the PR into `dev`, then move to the next step.
5. If a step is too big for one session, tell Claude: "Stop here, commit what works, and write what's left in `wl-docs/modules/<module>/dod.md`."

Tips that save a lot of pain:

* Ask for plan mode first on anything bigger than a small fix ("Plan this, don't write code yet").
* Keep Claude inside the step. If it starts touching other modules or upstream files, stop it.
* After each step, ask: "Run the review checklist from CLAUDE.md against this branch and list anything that fails."
* One module at a time. Don't start Booking while Practice is half done.

## Index

| File | Covers |
| --- | --- |
| [phase-0-foundation.md](phase-0-foundation.md) | Steps 0.1 to 0.10 (rules, repo setup, module kit, seams, example module, CI, webhook receiver, system docs) and the Phase 0 gate |
| [module-workflow.md](module-workflow.md) | Prompts A (docs first), B (build), C (review and close out), used for every module |
| [phase-1-practice-booking-calendar.md](phase-1-practice-booking-calendar.md) | 1.1 Practice, 1.2 Booking (done, now the built-in fallback); 1.3 Calendar dropped |
| [phase-2-messaging-platform.md](phase-2-messaging-platform.md) | Audiences and retargeting, consent (opt-in, STOP), ads attribution, one source of opening hours, templates |
| [phase-3-connectors-cliniko.md](phase-3-connectors-cliniko.md) | Cliniko spike, connector layer, Cliniko connector, privacy before the first real clinic |
| [phase-4-ai-agent.md](phase-4-ai-agent.md) | AI agent: knowledge base, booking through the connector, clinical handover, evals |
| [phase-5-reminders-recalls.md](phase-5-reminders-recalls.md) | Reminders and recalls driven by the connected system |
| [phase-6-saas.md](phase-6-saas.md) | Multi-business SaaS: model ADR, sign-up and onboarding, plans and billing, operator console |
| [phase-7-connectors-verticals.md](phase-7-connectors-verticals.md) | Vertical packs, calendar connectors, more practice systems, real estate |
| [recurring.md](recurring.md) | Upstream sync, release and bug fix prompts |

## Progress tracker

* [x] [0.1 SDLC doc in repo](phase-0-foundation.md#01-put-the-sdlc-doc-in-the-repo)
* [x] [0.2 CLAUDE.md](phase-0-foundation.md#02-create-claudemd)
* [x] [0.3 Remotes, branches, protection](phase-0-foundation.md#03-repo-and-branch-setup-you-run-these-not-claude)
* [x] [0.4 Module kit](phase-0-foundation.md#04-module-kit)
* [x] [0.5 Registry + Go seams](phase-0-foundation.md#05-registry-and-the-two-go-seams)
* [x] [0.6 Frontend index + Vue seams](phase-0-foundation.md#06-frontend-module-index-and-the-three-vue-seams)
* [x] [0.7 Example module](phase-0-foundation.md#07-example-module-end-to-end)
* [x] [0.8 CI with seam guard](phase-0-foundation.md#08-ci)
* [x] [0.9 Webhook receiver](phase-0-foundation.md#09-weline-webhook-receiver)
* [x] [0.10 System docs: architecture, data model, threat model](phase-0-foundation.md#010-system-docs-architecture-data-model-threat-model)
* [x] [Phase 0 gate](phase-0-foundation.md#phase-0-gate)
* [x] [1.1 Practice](phase-1-practice-booking-calendar.md#11-practice)
* [x] [1.2 Booking + chatbot flow](phase-1-practice-booking-calendar.md#12-booking)
* [x] ~~1.3 Calendar~~ (dropped by [ADR-002](../adr/ADR-002-connect-dont-build-pms.md))
* [x] [2.1 Audiences and retargeting](phase-2-messaging-platform.md#21-audiences-and-retargeting)
* [x] [2.2 Consent: opt-in and STOP](phase-2-messaging-platform.md#22-consent-opt-in-and-stop)
* [x] [2.3 Ads attribution (spike, then module)](phase-2-messaging-platform.md#23-click-to-whatsapp-ads-attribution)
* [x] [2.4 One source of opening hours](phase-2-messaging-platform.md#24-one-source-of-opening-hours)
* [ ] [WhatsApp templates submitted to Meta](phase-2-messaging-platform.md#whatsapp-templates-you-now)
* [ ] [Phase 2 gate](phase-2-messaging-platform.md#phase-2-gate)
* [ ] [3.0 Cliniko access and API spike](phase-3-connectors-cliniko.md#30-cliniko-access-and-api-spike) (from the published API, 2026-11-13; live checks wait for a trial key)
* [x] [3.1 Connector layer](phase-3-connectors-cliniko.md#31-connector-layer)
* [x] [3.2 Cliniko connector](phase-3-connectors-cliniko.md#32-cliniko-connector)
* [ ] [3.3 Privacy before the first real clinic](phase-3-connectors-cliniko.md#33-privacy-before-the-first-real-clinic) (audit trail, log scrubbing and booking retention built 2026-11-13; hosting waits on your choice, ADR-004)
* [ ] [Phase 3 gate](phase-3-connectors-cliniko.md#phase-3-gate)
* [ ] [Phase 4 AI agent](phase-4-ai-agent.md) (built 2026-11-13; real-model evals and the Cliniko staging check wait on keys)
* [ ] [Phase 5 Reminders and recalls](phase-5-reminders-recalls.md) (built 2026-11-13; the gate waits on the Cliniko trial, staging and approved templates)
* [ ] [Phase 6 Multi-business SaaS](phase-6-saas.md)
* [ ] [Phase 7 More connectors and vertical packs](phase-7-connectors-verticals.md)
