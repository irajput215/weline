# Calendly provider (step 7.2)

Spike: [`calendars-spike.md`](calendars-spike.md) (C1–C5). Code: `internal/modules/connectors/calendly/`,
`internal/modules/connectors/calendlycon.go`.

For businesses that already take bookings with Calendly. Calendly stays the system: event types are the services,
Calendly's availability is the free times, and **bookings and reschedules happen on Calendly's page**.

## Why bookings are by link

C3 is unconfirmed: Calendly's newer Scheduling API (creating a booking by API) couldn't be checked from the build
container, and it requires the invitee's **email**, which WhatsApp contacts don't have. Calendly's long-standing API
(read event types, availability, scheduled events and invitees; make one-time scheduling links; cancel) covers
everything else. Direct booking can be added later behind the same provider if C3 is confirmed and an email is asked
for in the chat.

## Requirements

- C-R1 **Connecting:** the business's personal access token (Calendly → Integrations → API and webhooks), tested by
  reading the token's user; stored encrypted like other credentials. No settings yet.
- C-R2 **Catalog:** services = the user's active event types (name, length); one practitioner (the token's user); one
  location "As set in Calendly" in the user's time zone.
- C-R3 **Free times:** Calendly's available times for the event type, read a week at a time (Calendly's limit) up to 28
  days or the number asked for.
- C-R4 **Booking by link:** the provider implements `provider.LinkBooker`: `BookingLink` makes a one-time scheduling
  link for the event type; `RescheduleLink` returns the invitee's own reschedule link. `Book` and `Reschedule` return
  "not supported". The AI agent sends the link instead of offering times ("You can choose a time and book here: …",
  outcome `sent_link`); if no link can be made, a person takes over.
- C-R5 **Customers by mobile:** Calendly has no customer records. A customer is `phone:<digits>`: from WhatsApp, the
  contact's number; from Calendly, the invitee's SMS reminder number, or an answer to a question mentioning "mobile" or
  "phone". Invitees without a number are `invitee:<uuid>` and get no reminders.
- C-R6 **Reading:** scheduled events (paged, only back to Calendly's API), with status (cancelled, completed if past)
  and the invitee as customer. Reminders and recalls work for invitees with a mobile.
- C-R7 **Cancel:** through Calendly's cancellation (Calendly tells the invitee). Confirm isn't supported.
- C-R8 **Errors:** bad token → "create a new token"; plan or permission → says so; throttling → try again; never raw
  responses.

## Limits

- Single user: the token owner's event types and events (organization-wide event types are later work).
- Weline's booking chatbot flow and public booking page offer times and then can't book with Calendly (the provider
  refuses); use the AI agent or send the Calendly link in a flow.
- The patient must type their mobile as Calendly's text reminder (or answer a mobile question) for reminders to reach
  them on WhatsApp; tell businesses to turn that on.

## Test plan

`calendly/calendly_test.go` against a fake Calendly: token refused, connection, catalog with paging, location time zone,
settings; free times in week windows; booking refused, one-time booking link, reschedule link, ids limited to UUIDs;
events across pages with status, customer from SMS number or question, `invitee:` fallback within 64 characters,
lookup by customer, mobile for reminders, cancel, unknown event, pagination refused off Calendly's API, throttling.
`connectors/calendly_test.go`: bad token refused, connected and resolved as a link booker. `ai/agent_test.go`: a booking
request gets the link and books nothing; no link → handover. Journey: Calendly can be chosen and asks for the token.

## DoD

- [x] C-R1–C-R8 implemented and tested.
- [ ] Checked with a real Calendly account (token, availability, link, reminder for an invitee with a mobile). **Needs a
  Calendly account; you.**
- [ ] C3 (Scheduling API) verified; if available, direct booking with an email question. Later.
