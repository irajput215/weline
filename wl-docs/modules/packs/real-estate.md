# Real estate on Weline (step 7.4)

How an agency runs on Weline today, with the [real estate pack](../../../internal/modules/packs/data/real_estate.json)
and the systems in [the spike](../connectors/real-estate-spike.md). A CRM connector comes later (spike decision 2).

## Set-up

1. **Sign up and choose "Real estate"** as the type of business (onboarding). The Real estate pack is recommended.
2. **Install the pack** (Get started → Industry packs): templates as drafts (inspection reminder, inspection follow-up,
   new listing, maintenance received), AI instructions with the price-guide and underquoting rules, emergency words for
   rentals (gas, fire, flooding…), three audiences, and, once Meta approves the templates, the inspection reminder
   (two hours before) and the week-later follow-up.
3. **Set up inspections** in Clinic and Bookings (Weline's names for its booking set-up):
   - each **listed property is a location** (its address as the name; remove it when it's sold or leased),
   - each **agent or property manager is a practitioner**, linked to the properties they show,
   - **services are inspection types**: "Private inspection" (15 minutes), "Open home" (30 minutes), "Appraisal"
     (60 minutes),
   - opening hours are when inspections can be booked.
4. **Choose where bookings go** (Connected system): Weline's built-in diary, or an agent's Google Calendar or Microsoft
   365 (busy times are respected), or Calendly (patients get a link).
5. **Turn on the AI agent** (AI → AI agent) with the property details in Knowledge (price guide, features, open home
   times); it answers only from that and books inspections.

## Flows

- **Book an inspection:** build it like the booking chatbot flow ([booking chatbot flow](../booking/chatbot-flow.md)),
  with "property" where it says "location".
- **Leads from portals and ads:** create an entry point per listing or campaign (Ads), with a tag such as
  `buyer-<suburb>`; enquiries arrive tagged and can be broadcast to later (with consent).
- **Maintenance requests (rentals):** the AI agent collects the address and problem and hands over to the property
  manager; send `wl_real_estate_maintenance_received` from the chat.

## What to tell agencies

- Weline never gives a price beyond the advertised guide, or legal or financial advice; it passes those to an agent.
- New-listing broadcasts go only to people who opted in (Consent), and every marketing message offers STOP.
- Their CRM stays their system of record; until a connector exists, inspections booked in Weline are visible in its
  booking screens or the connected calendar.
