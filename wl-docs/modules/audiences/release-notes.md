# Audiences: release notes

## 2.1: Segments and retargeting (first release)

Shipped in Weline Phase 2.1. Spec: [`requirements.md`](requirements.md). Design: [`design.md`](design.md).
API: [`api.md`](api.md). Feeds Weline's existing **Campaigns**, which still send, schedule and track every message.

### What's new

An **Audiences** section in the sidebar with Segments, Retarget and History.

- **Segments:** save who a broadcast is for as rules, matched as *all* or *any* (up to 20):
  - has or doesn't have a **tag**;
  - a **contact field** is, isn't, contains, is set or is empty (fields are suggested from your contacts);
  - is on a given **WhatsApp number**;
  - **last wrote to us** within or not within N days (up to 365);
  - was **added** within or not within N days;
  - **received**, didn't receive, read, didn't read, read but didn't reply to, or replied to a past campaign.

  A segment is worked out again each time it's used, so new contacts who match are included. Segments can be
  renamed, edited, duplicated and deleted. Deleting one keeps the history of campaigns made from it.
- **Live count while you build:** how many contacts match, how many would get the message, how many were left out
  and why (duplicate numbers, no usable number, over the limit, and opted out of marketing), plus up to 20 examples
  with masked numbers.
- **Create campaign:** from a saved segment:
  - choose a name, the WhatsApp number and an approved template on that number;
  - fill each template variable from the contact's name, phone, a contact field or fixed text, with a fallback when
    it's empty, and see the message as the first few people would get it;
  - confirm the recipient count. If the audience changed in the meantime, you're asked to check again.

  This creates a **draft** in Campaigns, where you add media, schedule it or start it.
- **Retarget:** pick a finished campaign (sent, delivered, read and failed counts shown) and follow up the people who
  didn't receive it, didn't read it, read it but didn't reply within N days, or replied. The follow-up is always a
  new draft, never a resend of the same campaign, and can also be saved as a segment.
- **History:** every campaign created from Audiences, whether it came from a segment or a retarget, how many
  recipients, how many were left out, who created it and when, with a link to the campaign.
- **Safe by default:** each number appears once per campaign; contacts who opted out of marketing are left out
  whenever the template is a marketing one (Weline's sender checks again when it sends); at most 10,000 recipients
  per campaign, most recently active first.

### Who can do what

| Permission | Admin | Manager | Agent (reception) |
| --- | --- | --- | --- |
| `wl_audiences_segments` (view segments, counts and history; create, edit, delete) | read, create, update, delete | read, create, update, delete | read |
| `wl_audiences_sends` (turn a segment or retarget into a draft campaign) | create | create | – |

Creating a campaign also needs Weline's own **Campaigns: write** permission, and starting it needs **Campaigns:
execute**, as before.

### Upgrade notes

- Migration `audiences/0001_create_audiences` adds `wl.audiences_segments` and `wl.audiences_sends`, and grants the
  permissions above to the default roles. It doesn't change any Weline table.
- Organizations created after the upgrade need the `wl_audiences_*` permissions granted in the role editor (same as
  other modules).
- No WhatsApp templates of its own: it uses the templates you've had approved by Meta.

### Known limits

- No recorded opt-in yet: contacts are left out only if they opted out of marketing. Opt-in and STOP come in step 2.2
  (Consent).
- No segments based on appointments or visits (needs the practice-system connector, Phase 3), no cost estimate,
  and no recurring campaigns.
- The sidebar shows Audiences as its own section, not under Messaging.
