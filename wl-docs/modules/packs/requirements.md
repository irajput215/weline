# Industry packs: requirements

**For:** a business setting up Weline (owner or manager), who wants the templates, AI setup, audiences and reminder
rules that suit their industry without writing them. Build guide step 7.1
([`../../build-guide/phase-7-connectors-verticals.md`](../../build-guide/phase-7-connectors-verticals.md)).

Status: **built; pack wording to be reviewed by you and a clinic.**

## User stories

- As a practice manager, after choosing "allied health" in onboarding, I see the Clinics pack recommended, look at
  what's in it, and install it in one click.
- As an owner who already wrote my own AI instructions, installing a pack never overwrites them.
- After Meta approves the templates, I install again and the reminder rules appear, switched off until I turn them on.

## Functional requirements

- R1 **A pack is data, not code:** a JSON file per industry in `internal/modules/packs/data/`, checked when Weline
  starts and in tests (unknown fields refused). Contents: templates, AI tone, instructions and emergency words,
  suggested segments (Audiences rules), reminder and recall rules (each naming one of the pack's templates), chatbot
  flow guides, the connectors that fit, and which onboarding verticals it suits.
- R2 **Template rules:** names lowercase and prefixed `wl_<pack>_`; category UTILITY or MARKETING; marketing templates
  must say how to stop; variables `{{1}}…` in order with one sample each; at most 3 quick-reply buttons.
- R3 **Install** (idempotent, never changes what exists):
  - templates are created as **drafts** on the business's default WhatsApp number, with buttons and samples, to submit
    to Meta from Templates; without a number they wait;
  - AI: the pack's instructions (and tone) only if the business has none; emergency words added to theirs without
    duplicates; the agent's on/off is never changed;
  - segments created by name if missing, through Audiences (so its validation applies);
  - reminder and recall rules created **off**, once their template is approved; until then reported as waiting;
  - each install recorded with what it did, and audited.
- R4 **Recommendation:** the pack for the vertical chosen in onboarding is listed first and marked; onboarding's
  type-of-business step links to the packs page.
- R5 **Packs:** Clinics (allied health, dental, medical), Beauty and wellness, Fitness and studios, Real estate (7.4:
  inspections, follow-ups, new listings, maintenance; [set-up guide](real-estate.md)), Small business (other). Every
  onboarding vertical has one (tested).
- R6 **Permissions:** `wl_packs_install` read and create for admin and manager.

## Not in this step

- Importing chatbot flows automatically: packs name the flow and point to its build guide (the booking flow guide);
  flows are built in Weline's flow builder until an import format is agreed.
- Knowledge base entries: the AI agent answers only from each business's own information, so packs don't add FAQs.

## Open questions

- Q1 Review the pack wording (especially the clinic AI instructions and emergency words) with a clinic. **Default: as
  written.**
- Q2 Should installing submit templates to Meta straight away? **Default: no, drafts; the business reviews and submits.**
