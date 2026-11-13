# Onboarding: release notes

## Step 6.1: self-serve sign-up and the setup checklist (first release)

Spec: [`requirements.md`](requirements.md). Design: [`design.md`](design.md). API: [`api.md`](api.md).

### What's new

- **Sign up** at `/x/onboarding/signup` (when you turn it on with `WELINE_SIGNUP=open` and the Turnstile keys): a
  business gets its own Weline with the person signing up as admin.
- **Get started → Setup checklist** for admins and managers: business details, WhatsApp number, your system, type of
  business, team and first message, each linking to where it's done and ticking itself off. Hide it when you're done.
- **Source code link** (`/x/source`) to the exact version running, as the AGPL asks of a hosted service.

### Fixed

- Organizations created after Weline's modules were installed (with "create organization") now get the module
  permissions for their manager and reception roles. Existing organizations get any they were missing once; if you
  removed a module permission from a system role before this release, check that role afterwards.
