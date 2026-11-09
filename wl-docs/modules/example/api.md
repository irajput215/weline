# Example: API

### GET /api/x/example/notes
Permission: wl_example_notes:read
Request: none
Response 200: { status: "success", data: { notes: [ { id, organization_id, text, created_by_id, created_at, updated_at } ] } } (newest first, max 100)
Errors: 401 not logged in · 403 no permission

### POST /api/x/example/notes
Permission: wl_example_notes:create
Request: { text }
Response 200: { status: "success", data: { note } }
Errors: 400 "note text is required" / "note text must be at most 1000 characters" / invalid body · 401 not logged in · 403 no permission
