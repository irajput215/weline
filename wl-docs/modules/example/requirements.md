# Example: requirements

**For:** developers. This module exists to prove the module pattern end to end (build guide step 0.7) and to be copied as the starting point for real modules. Delete it once real modules exist, or keep it as a template.

## User stories
- As a clinic staff member, I add a short note for my team so that it is visible to colleagues in my clinic.
- As a clinic staff member, I see my clinic's notes, newest first, and never another clinic's.

## Functional requirements
- R1 Staff with `wl_example_notes:read` see their organization's notes, newest first, at most 100.
- R2 Staff with `wl_example_notes:create` add a note of 1 to 1000 characters (trimmed).
- R3 Notes are scoped to the active organization; another organization never sees or changes them.
- R4 Without read permission the page and its sidebar entry are hidden and the API returns 403; with read but not create, the add form is replaced by an explanation and the API refuses to create.
- R5 New permissions go to each existing organization's system `admin` role on first install.

## Non-functional
- Every screen state: loading, empty, error, forbidden.
- Note text is staff-written content: never logged.

## Out of scope (this slice)
- Editing or deleting notes, attachments, mentions.
