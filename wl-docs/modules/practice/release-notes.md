# Practice: release notes

## 2.4: One source of opening hours

- **Whether you're open now comes from the chatbot's business hours** (Settings → Chatbot), the same hours the
  out-of-hours reply on WhatsApp uses. Set your hours there once.
- **Clinic → Opening hours is now Booking hours.** It only decides when patients can book with built-in booking. The
  page says so and links to the chatbot's business hours.
- **For other modules:** `IsOpen(ctx, orgID, account, at)` now reads the chatbot business hours (the WhatsApp number's
  own settings first, then the organization's) in the clinic's time zone. It no longer takes a location.
- **Check on the server:** Weline's own out-of-hours reply uses the server's clock. Set the server's `TZ` to your
  clinic's zone (for example `Australia/Sydney`), or the reply goes out at the wrong times. Staging has `TZ=UTC`.
- Closures (public holidays) still apply to booking only. On a public holiday, change the chatbot's hours for the day.

## 1.1: Clinic setup (first release)

Shipped in Weline Phase 1.1. Spec: [`requirements.md`](requirements.md). Design: [`design.md`](design.md).
API: [`api.md`](api.md).

### What's new

A **Clinic** section in the sidebar where a practice describes itself. Booking (1.2), Calendar (1.3) and the
later modules read it.

- **Practice profile:** name, time zone (one of the seven Australian zones), booking link name
  (`/book/<slug>`, with a live preview), appointment grid (5, 10, 15, 20, 30 or 60 minutes), and optional ABN
  (checksum verified), phone, email and website. While setup is incomplete, a checklist on this page shows what is
  still missing.
- **Locations:** name, address and phone; deactivate and reactivate (never deleted).
- **Practitioners:** name, role, optional AHPRA number, optional link to a Weline login (one practitioner per
  user, members of this organization only), and the locations they work at.
- **Services:** name, duration in whole slots (up to 8 hours), optional price in dollars (incl. GST), and the
  practitioner roles that can provide it. A warning shows when no active practitioner has one of the roles yet.
- **Opening hours:** per location, closed or one opening block per weekday, on the appointment grid.
- **Closures:** whole days for one location or all, between two dates, with a reason (e.g. Good Friday).
  Closures from the last 12 months stay listed, marked as past.
- Changing the appointment grid is refused if any service or opening time would fall off it; the message names
  them, so nothing is rounded silently.

### Who can do what

| Permission | Admin | Manager | Agent (reception) |
| --- | --- | --- | --- |
| `wl_practice_settings` (profile, hours, closures) | read, update | read, update | read |
| `wl_practice_locations` | read, create, update | read, create, update | read |
| `wl_practice_practitioners` | read, create, update | read, create, update | read |
| `wl_practice_services` | read, create, update | read, create, update | read |

Reception sees every page with no edit controls. Change the grants in Weline's role editor.

### Upgrade notes

- **Migration:** `practice/0001_create_practice` (seven new `wl.practice_*` tables, 11 permissions). Additive; it
  runs on deploy with `-migrate`. Nothing in upstream tables changes.
- **Organizations created after this deploy** don't get the `wl_practice_*` grants automatically (upstream seeds
  their roles). An admin grants them in the role editor; until then their staff see no Clinic menu.
- No configuration, secrets or WhatsApp templates needed.

### For other modules

`internal/modules/practice` offers `GetProfile` (`IsNotSetUp(err)` until saved), `ActiveLocations`,
`ActivePractitioners` (with their locations), `ActiveServices` and `IsOpen(ctx, orgID, locationID, at)`, which
applies the clinic's time zone (daylight saving included), weekday hours and closures.

### Known limitations

- One time zone, one booking link and one opening block per day for each clinic (no lunch breaks or
  alternating weeks); fixed duration and price per service. These are the defaults for open questions Q2–Q6.
- Public holidays are entered by hand as closures.
- Changes to setup are not audited yet (threat model, open risk 7).
- English only.
