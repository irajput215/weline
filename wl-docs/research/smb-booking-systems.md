# Research: booking systems of appointment-based small businesses

Date: 2026-11-21 · Status: desk research (build guide step 8.0); no sandbox or live tests yet

**Question:** after clinics, Weline's next customers are appointment-based small businesses (salons, barbers, beauty,
fitness studios, trades). Under [ADR-002](../adr/ADR-002-connect-dont-build-pms.md) we connect to the system they already
book in. Which systems do Australian businesses use, and which can Weline connect to?

**Answer:** connect **Square Appointments first**: popular with Australian salons and barbers, free for one location,
a public Bookings API available in Australia with availability search, customers and **webhooks**, and the same account
holds payments (so automatic receipts become possible later). The two most-used salon systems, **Fresha** and
**Timely**, have no self-serve API: they need a partner conversation. **Acuity** is an easy second connector;
**Mindbody** suits fitness but access costs and needs approval; **ServiceM8** is the trades choice. Businesses with no
system keep Weline's built-in booking or the Google, Microsoft and Calendly connectors that already exist.

## Systems

| System | Who uses it (AU) | API | Webhooks | Auth | Verdict |
| --- | --- | --- | --- | --- | --- |
| **Square Appointments** | Salons, barbers, beauty, cleaners; strong where the business already takes cards with Square. Free (1 location), Plus $40, Premium $90 a location a month | Public Bookings API, GA since 2022 incl. **Australia**: availability search (`POST /v2/bookings/availability/search`), create, update, cancel bookings; Customers API; Catalog for services; Team for staff | **Yes**: `booking.created`, `booking.updated` (seller-level permission sees every booking, incl. ones made on Square's own booking site) | OAuth (seller authorises Weline) | **First connector.** To confirm in the sandbox: whether seller-level *write* needs Plus or Premium; webhook gaps reported in Square's forum |
| **Fresha** | Probably the most used salon software in Australia (free to businesses, marketplace) | **No public REST API**; a paid read-only data export to Snowflake; a public MCP server for AI assistants to discover and book | No | Partner terms only | **Ask Fresha** about partner access. Can't connect today |
| **Timely** (NZ/AU) | Very common with AU/NZ salons | A REST API for paid plans keyed by an admin token, reported by third parties; no public portal, OAuth or docs | Unknown | Admin token | **Ask Timely** for partner access and documents |
| **Acuity Scheduling** (Squarespace) | Solo practitioners, coaches, some beauty and wellness | REST: availability dates and times, appointments, types, calendars | **Yes**: scheduled, rescheduled, cancelled, signed with HMAC-SHA256 (`x-acuity-signature`); up to 25 dynamic webhooks per account | Basic auth (user id + API key) or OAuth | **Good second**, small effort; close to the Calendly connector |
| **Mindbody** | Gyms, yoga and pilates studios | Public API v6 (`API-Key` + `SiteId`); free under 5,000 calls a billing cycle, paid above; the business's plan must include API access | **Yes**, separate approval for live access | API key per developer + site activation | **Later, for fitness**; check cost per studio and approval time |
| **ServiceM8** | Trades (plumbers, electricians, cleaners) in AU | REST API, jobs, clients, staff | **Yes**, only for public OAuth apps; users report bugs in the newer event webhooks | OAuth 2.0 (1-hour tokens, refresh) | **Later, for trades**; jobs rather than appointment slots, so a different booking flow |
| Others seen in AU comparisons: Vagaro, Booksy, GlossGenius, Phorest, Glofox, Setmore, SimplyBook.me | Smaller AU shares | Not researched yet | | | Only if a customer asks |

## Gaps in Weline for these businesses (from the code, 2026-11-21)

- **Wording is clinic-only.** The Booking, Practice and Reminders screens say "patient", "practitioner" and "clinic"
  (about 50 strings, e.g. `booking/i18n/en.json`, `practice/i18n/en.json`). A salon sees "Add patient". Packs already
  exist for beauty, fitness and general businesses, but the screens don't follow the organization's vertical.
- **Packs offer only the built-in diary** to beauty, fitness and general businesses, although Google Calendar,
  Microsoft 365 and Calendly connectors exist (only the real estate pack lists them).
- **No trades vertical** in onboarding (`allied_health`, `dental`, `medical`, `beauty`, `fitness`, `real_estate`,
  `other`).
- **Connectors poll** (Cliniko has no webhooks). Square, Acuity, Mindbody and ServiceM8 push changes, so the connector
  layer needs a way to receive provider webhooks: a public receiver per provider, verified by signature, like the
  hooks receiver.

## Next steps (build guide [phase 8](../build-guide/phase-8-appointment-smbs.md))

1. 8.1 Wording follows the business type (no connector needed, helps every non-clinic customer now).
2. 8.2 Square Appointments connector: sandbox spike first (OAuth app, scopes and plan needed for write, webhook
   reliability), then the provider.
3. 8.3 Packs offer the connectors that fit (calendars, Square); a trades vertical when there's a pilot.
4. **You:** ask Fresha and Timely about partner API access; ask five salon or studio owners which message they send by
   hand most often.

## Sources

- Popularity: [wmtips Fresha](https://www.wmtips.com/technologies/scheduling/fresha/),
  [Fresha vs Timely](https://www.wmtips.com/technologies/compare/fresha-vs-timely/),
  [Fresha vs Square Appointments](https://www.wmtips.com/technologies/compare/fresha-vs-square-appointments/),
  [Capterra AU salon software](https://capterra.com.au/directory/30028/salon/software?page=2),
  [GetApp AU salon software](https://www.getapp.com.au/directory/212/salon/software)
- Square: [Bookings API](https://developer.squareup.com/docs/bookings-api/what-it-is),
  [Bookings webhooks](https://developer.squareup.com/docs/bookings-api/use-webhooks),
  [booking.created](https://developer.squareup.com/reference/square/bookings-api/webhooks/booking.created),
  [GA announcement](https://developer.squareup.com/blog/build-with-bookings-api-now-generally-available/),
  [forum: missing booking.created](https://developer.squareup.com/forums/t/webhook-booking-created-events-are-missing-for-some-appointments/27157.md),
  [AU pricing](https://squareup.com/help/au/article/6359)
- Fresha: [API Evangelist](https://providers.apievangelist.com/providers/fresha/),
  [Supergood](https://supergood.ai/docs/fresha-api),
  [CX Network on the MCP server](https://www.cxnetwork.com/cx-experience/articles/fresha-goes-conversational-with-native-claude-and-chatgpt-integration-for-beauty-bookings),
  [partner terms](https://terms.fresha.com/partner-terms)
- Timely: [Supergood report card](https://supergood.ai/api-report-card/timely),
  [Truto](https://truto.one/integrations/detail/timely/)
- Acuity: [webhooks](https://developers.acuityscheduling.com/docs/webhooks),
  [dynamic webhooks](https://developers.acuityscheduling.com/page/webhooks-webhooks-webhooks)
- Mindbody: [developer tools](https://www.mindbodyonline.com/es-es/node/4140),
  [webhooks](https://developers.mindbodyonline.com/WebhooksDocumentation),
  [usecarly guide](https://www.usecarly.com/blog/mindbody-api/)
- ServiceM8: [webhooks](https://developer.servicem8.com/docs/webhooks-overview),
  [authentication](https://developer.servicem8.com/docs/authentication),
  [add-on types](https://developer.servicem8.com/docs/add-on-types)
