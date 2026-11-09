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
| [module-workflow.md](module-workflow.md) | Prompts A (docs first), B (build), C (review and close out), used for every module in Phases 1 to 5 |
| [phase-1-practice-booking-calendar.md](phase-1-practice-booking-calendar.md) | 1.1 Practice, 1.2 Booking (extras, chatbot flow), 1.3 Calendar, WhatsApp templates, Phase 1 gate |
| [phase-2-reminders.md](phase-2-reminders.md) | Reminders extras and Phase 2 gate |
| [phase-3-ai-responses.md](phase-3-ai-responses.md) | AI responses extras and Phase 3 gate |
| [phase-4-cliniko-connector.md](phase-4-cliniko-connector.md) | Cliniko connector extras and Phase 4 gate |
| [phase-5-crm-recalls.md](phase-5-crm-recalls.md) | CRM and recalls scope and Phase 5 gate |
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
* [ ] [0.9 Webhook receiver](phase-0-foundation.md#09-weline-webhook-receiver)
* [x] [0.10 System docs: architecture, data model, threat model](phase-0-foundation.md#010-system-docs-architecture-data-model-threat-model)
* [ ] [Phase 0 gate](phase-0-foundation.md#phase-0-gate)
* [ ] [1.1 Practice](phase-1-practice-booking-calendar.md#11-practice)
* [ ] [1.2 Booking + chatbot flow](phase-1-practice-booking-calendar.md#12-booking)
* [ ] [1.3 Calendar](phase-1-practice-booking-calendar.md#13-calendar)
* [ ] [WhatsApp templates submitted to Meta](phase-1-practice-booking-calendar.md#phase-1-gate)
* [ ] [Phase 1 gate](phase-1-practice-booking-calendar.md#phase-1-gate)
* [ ] [Phase 2 Reminders](phase-2-reminders.md)
* [ ] [Phase 3 AI responses](phase-3-ai-responses.md)
* [ ] [Phase 4 Cliniko connector](phase-4-cliniko-connector.md)
* [ ] [Phase 5 CRM and recalls](phase-5-crm-recalls.md)
