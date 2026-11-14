# Calendar connectors: spike (step 7.2)

Date: 2026-11-14. Written from the providers' published APIs as of mid-2026; their documentation sites weren't
reachable from the build container, so **items marked (verify) must be checked against the live docs and a test
account** before each provider goes to a real business (as with the Cliniko spike).

For small businesses without a special system: their calendar is the "system" (ADR-002). Weline keeps the business
setup it already has (services and lengths, locations, staff, opening hours: the practice module) and uses the
calendar for **busy times and bookings**.

## Google Calendar (API v3)

| Question | Finding |
| --- | --- |
| G1 Access without Google approving a Weline app? | **Yes, with a service account.** The business shares its calendar with Weline's service account address ("Make changes to events"). Works for personal Gmail and Workspace calendars. User OAuth would need Google's verification of a "sensitive" scope (calendar) before going past test users: weeks, a privacy policy and a demo video (verify current process). |
| G2 Token | Service account JWT (RS256, `iss` = service account email, `scope` = `https://www.googleapis.com/auth/calendar`, `aud` = token URI) exchanged at `https://oauth2.googleapis.com/token`; 1-hour access tokens. |
| G3 Busy times | `POST /calendar/v3/freeBusy` with `timeMin`, `timeMax`, `items[{id}]` → busy intervals per calendar. Needs read access to the calendar (sharing gives it). |
| G4 Book / move / cancel / read | `POST /calendars/{id}/events`, `PATCH …/events/{eventId}`, `DELETE …/events/{eventId}` (event becomes `status: cancelled`), `GET …/events/{eventId}`, `GET …/events` (`timeMin`/`timeMax`, `updatedMin` with `showDeleted`, `singleEvents=true`, `privateExtendedProperty=key=value` filters, paged by `nextPageToken`). |
| G5 Linking an event to the Weline contact | `extendedProperties.private` (key/values only Weline's project sees): `weline=1`, contact, service, location, practitioner. |
| G6 Limits | Per-project daily quota (default about 1,000,000 queries) and per-user per-minute limits; `403 rateLimitExceeded` / `429` → back off (verify current numbers). |
| G7 Attendee invitations | Service accounts can't invite attendees without Workspace domain-wide delegation. Not needed: the patient is told on WhatsApp. |
| G8 Change notifications | Push channels (`events.watch`) need a verified HTTPS domain and renewing; not needed because Weline reads the calendar live when it searches, books or sends reminders. |

## Microsoft 365 / Outlook (Microsoft Graph)

| Question | Finding |
| --- | --- |
| M1 Access | **App-only** (client credentials) with application permission `Calendars.ReadWrite`, granted once by the business's Microsoft 365 admin through an admin-consent link; limit the app to the booking mailboxes with an application access policy / RBAC for applications (verify current mechanism). Delegated OAuth for many tenants needs publisher verification. Personal Outlook.com accounts can't use app-only. |
| M2 Busy times | `POST /users/{id}/calendar/getSchedule` (free/busy for mailboxes, time window and interval). |
| M3 Book / move / cancel / read | `POST /users/{id}/events`, `PATCH /users/{id}/events/{eventId}`, `POST …/cancel` or `DELETE`, `GET …/calendarView?startDateTime&endDateTime`; link via `singleValueExtendedProperties` or open extensions. |
| M4 Limits | Outlook throttling per app per mailbox (about 10,000 requests per 10 minutes) with `429` + `Retry-After` (verify). |
| M5 Needs from you | An Azure (Entra) app registration for Weline (client id and secret or certificate), the admin-consent link, and a test Microsoft 365 tenant. |

## Calendly (API v2)

| Question | Finding |
| --- | --- |
| C1 Access | Personal access token per business, or OAuth app (Calendly app registration). |
| C2 Availability | `GET /event_type_available_times` for an event type, in windows of at most 7 days; `GET /user_busy_times` (verify). |
| C3 Booking by API | Historically **not possible**: bookings happen on Calendly's page. Calendly announced a Scheduling API (create an invitee by API) in 2025 (**verify** availability and which plans). Without it, Weline sends the business's Calendly link and learns of bookings afterwards. |
| C4 Bookings made | `GET /scheduled_events` (by user/organization, time range) and webhooks `invitee.created` / `invitee.canceled` (paid plans; verify). |
| C5 Fit | Read side (reminders, recalls) fits the connector interface; the booking side depends on C3. |

## Decision

1. **Build Google Calendar now** with the service-account model (no Google app review, works for most small businesses).
   Weline's catalog and opening hours give candidate times; the calendar's busy times are removed; bookings are events
   tagged with the Weline contact; reminders and recalls read those events.
2. **Microsoft 365 next**, once an Entra app registration exists (M5). Same shape: catalog in Weline, `getSchedule`
   for busy times, events for bookings.
3. **Calendly after confirming C3.** With the Scheduling API, a full provider; without it, a read-only provider for
   reminders plus "send the booking link" in the chatbot.

## Needs from you

- A Google Cloud project for Weline with the Calendar API enabled and a **service account key** (JSON) put in the
  host's environment as `WELINE_GOOGLE_SERVICE_ACCOUNT` (secret).
- For Microsoft: an Entra app registration and a test tenant. For Calendly: a test account on a paid plan.
