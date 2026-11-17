# Contact timeline: design

Spec: [`requirements.md`](requirements.md). Code: `internal/modules/timeline/`, `frontend/src/modules/timeline/`.

- **No tables.** One query per source, each filtered by organization and contact (broadcast recipients by the
  contact's number through campaigns of the organization, since recipients have no organization column), each limited
  to a page and to items before the cursor; merged and sorted in Go; the 100th item's time is the next cursor.
- Sources and their columns are a table in `service.go` (`sources`); adding a module's events is a new row.
- **Appointments** come from the organization's provider (the module is a `provider.Consumer`): the customer is found
  with `provider.Finder` when the provider has it (Cliniko), otherwise `FindOrCreateCustomer` (the built-in diary and
  calendar providers only look up the Weline contact; Calendly matches the mobile). A not-found customer means no
  appointments; any other error sets `appointments_unavailable`.
- The first page reads appointments up to a year ahead, so upcoming ones are on top; later pages only go backwards.
