# Onboarding: requirements

**For:** a business owner who finds Weline and wants to start without talking to us, and the owner or manager of an
existing organization finishing setup. Build guide step 6.1
([`../../build-guide/phase-6-saas.md`](../../build-guide/phase-6-saas.md)); model in
[ADR-005](../../adr/ADR-005-saas-model.md).

Status: **built on the defaults below; open questions still to confirm.**

## User stories

- As a business owner, I sign up with my business name, my name, email and password, and get my own Weline
  organization where I'm the admin.
- As a new admin, I see a checklist: business details, connect WhatsApp, connect my system, choose my type of business,
  invite my team, send a first message. Each step opens the screen where it's done, and ticks itself off once done.
- As a manager of an existing organization, I see the same checklist and can hide it.

## Functional requirements

- R1 **Public sign-up page** at `/x/onboarding/signup` (no login), **off unless `WELINE_SIGNUP=open`**. Fields: business
  name (2–120), your name (2–120), email, password (at least 12 characters, upstream's rule), time zone (the practice
  module's list, default `Australia/Sydney`), accepting the terms and privacy policy. Protected by the Turnstile robot
  check (refused without keys) and 5 sign-ups an hour per address.
- R2 **Sign-up creates**, in one transaction: an organization with a unique slug, upstream's system roles and chatbot
  settings (as upstream's "create organization" does), the user as **admin** with that organization as default, the
  practice profile (name, time zone, booking page address), the onboarding state, and an audit entry. An email that
  already has an account is refused (sign in instead). The person then signs in on the normal login page.
- R3 **Module permissions for new organizations:** organizations created after a module's migration (by sign-up or by
  upstream's "create organization") get the module permissions their manager and agent roles should have, once per
  organization: straight after a sign-up, and every 5 minutes for others.
- R4 **Checklist**, worked out from what exists, never stored: business details (practice profile), WhatsApp number
  (a WhatsApp account), system (a connected connector), type of business (chosen), team (a second member), first
  message (an outgoing message). Each step links to the existing screen.
- R5 **Type of business:** allied health, dental, medical, beauty, fitness, real estate (7.4, migration `0002`), other. Stored now; Phase 7's vertical packs
  use it.
- R6 **Hide / show** the checklist.
- R7 **Permissions:** `wl_onboarding_setup` read and update for admin and manager; reception doesn't see it.
- R8 **AGPL source link** (ADR-005): `/x/source` redirects to the source of the running build (the commit Go recorded,
  or `WELINE_SOURCE_URL`); the sign-up page links to it.

## Non-functional

- No passwords, names or emails in logs; the password is hashed with bcrypt like upstream.
- Every query filters by organization; sign-up writes only the new organization's rows.

## Open questions (built on the default shown)

- Q1 Verify email addresses before the account works? **Default: no (Weline has no email sending yet); the robot check
  and rate limit apply. Add verification when email is set up.**
- Q2 Where do the terms of service and privacy policy live? **Default: `WELINE_TERMS_URL` / `WELINE_PRIVACY_URL`;
  placeholders until you publish them.**
- Q3 Should a new organization start with the built-in diary already chosen? **Default: no; the "connect your system"
  step asks.**
