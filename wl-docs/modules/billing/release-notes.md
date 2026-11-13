# Billing: release notes

## Step 6.2: plans, limits and billing (first release)

Spec: [`requirements.md`](requirements.md). Design: [`design.md`](design.md). API: [`api.md`](api.md).

### What's new (when billing is turned on with `WELINE_BILLING=on`)

- **Billing → Plan & billing:** your plan and its status, the trial end or renewal date, this month's usage against
  the plan, the plans you can choose, invoices, and how many template messages Meta will charge you for.
- **Choose a plan** pays by card on Stripe's secure page; **Manage billing** opens Stripe's page to change card or
  plan, cancel, and download invoices. Admins only; managers can look.
- **Limits:** broadcasts beyond the month's allowance are refused with a message saying so; once the AI agent's
  monthly replies are used up, chats go to your team instead. Reminders, replies to patients and STOP are never limited.
- New businesses get a 14-day trial without a card.

With billing off (the default), nothing changes except that the page shows usage.
