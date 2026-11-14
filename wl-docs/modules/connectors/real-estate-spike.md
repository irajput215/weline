# Real estate: spike (step 7.4)

Date: 2026-11-14. Written from published information known as of mid-2026. None of these vendors' sites or APIs were
reachable from the build container, and every CRM below requires a partner or integration agreement for API access,
so **everything marked (verify) must be confirmed with the vendor before building a connector.**

## Which systems Australian agencies use

| System | Typical users | API (verify) | Access (verify) |
| --- | --- | --- | --- |
| **Rex** (Rex Software) | Sales and property management, many independents and franchise groups | Rex API: authenticated JSON API over contacts, listings, open homes/inspection appointments, leads | Partner program; per-agency authorisation |
| **VaultRE** (now part of MRI Software, verify) | Sales-focused agencies, strong among independents | REST API (versioned, API key plus per-agency access token): contacts, properties, open homes, enquiries | Integration partner registration; agency enables the integration |
| **Agentbox** (part of Reapit, verify) | Sales and franchise networks | REST API: contacts, listings, inspections, enquiries | Integration partner approval |
| **Box+Dice**, **MRI Sales / Property Tree** (property management), **PropertyMe** (property management) | Varies; PropertyMe and Property Tree are common for rentals | PropertyMe has a public API for property management (verify) | Varies |
| **Portals**: realestate.com.au, Domain | Where buyer enquiries start | Domain: developer API (listings, inspections; verify plans); REA: partner lead feeds | Developer or partner terms |

## What a connector would map

Weline's connector interface is appointment-centred, which fits **inspections**:

| Weline | Real estate |
| --- | --- |
| Location | A listed property (address) |
| Practitioner | An agent / property manager |
| Service | Inspection type (private inspection, open home slot, appraisal) |
| Free times | Open-home slots and agents' private-inspection availability |
| Customer | The buyer/tenant contact (matched by mobile) |
| Appointment | An inspection booking |

Also useful later, outside the interface: **lead capture** (portal enquiries arriving as WhatsApp conversations: the
Ads module's entry points already tag them), **new-listing broadcasts** (Audiences), and **maintenance requests** for
property management (the AI agent's handover with notes).

## Decision

1. **Ship the real estate industry now on what works and is tested:** the onboarding type "Real estate", the
   [real estate pack](../packs/real-estate.md) (templates, AI guardrails including underquoting, segments, inspection
   reminders and follow-ups), and inspections through Weline's built-in diary (properties as locations, agents as
   staff, inspection types as services) or an agent's Google/Microsoft calendar or Calendly. This meets the Phase 7
   gate's wording: a non-clinic business on a connector with its pack.
2. **Build one CRM connector when an agency customer asks and grants sandbox access**, starting with the CRM that
   agency uses (Rex or VaultRE are the likeliest). Each needs a partner agreement and recorded fixtures, as with
   Cliniko.
3. **Portals:** treat portal enquiries as leads into WhatsApp via entry points first; a Domain API integration only if
   agencies ask.

## Needs from you

- An agency willing to pilot, and which CRM it uses; then Weline applies to that CRM's partner program (API keys,
  sandbox, terms).
- Legal check of the pack's AI guardrails against state underquoting and privacy rules (NSW, VIC and QLD differ).
