package ai

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/fastglue"
)

type fakeAuth struct{ allowed map[string]bool }

func (f fakeAuth) OrgIDFromRequest(r *fastglue.Request) (uuid.UUID, error) {
	id, _ := r.RequestCtx.UserValue("organization_id").(uuid.UUID)
	return id, nil
}
func (f fakeAuth) HasPermission(_ uuid.UUID, res, act string, _ ...uuid.UUID) bool {
	return f.allowed[res+":"+act]
}

func perms(keys ...string) fakeAuth {
	m := map[string]bool{}
	for _, k := range keys {
		m[k] = true
	}
	return fakeAuth{allowed: m}
}

var (
	manager = perms(PermSettings+":read", PermSettings+":update", PermKnowledge+":read", PermKnowledge+":create", PermKnowledge+":delete", PermRuns+":read")
	staff   = perms(PermRuns + ":read")
	nobody  = perms()
)

func (f *fixture) call(t *testing.T, auth kit.Auth, method, path, id string, body any, org uuid.UUID) (int, map[string]any) {
	t.Helper()
	var h fastglue.FastRequestHandler
	for _, rt := range routes(auth, f.svc, f.db) {
		if rt.method == method && rt.path == path {
			h = rt.handler
		}
	}
	require.NotNil(t, h, "%s %s", method, path)
	rq := testutil.NewRequest(t)
	if body != nil {
		b, _ := json.Marshal(body)
		rq.RequestCtx.Request.SetBody(b)
	}
	if id != "" {
		rq.RequestCtx.SetUserValue("id", id)
	}
	if org != uuid.Nil {
		testutil.SetAuthContext(rq, org, f.user)
	}
	require.NoError(t, h(rq))
	var env struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rq.RequestCtx.Response.Body(), &env)
	return rq.RequestCtx.Response.StatusCode(), env.Data
}

func TestHandlers(t *testing.T) {
	f := newFixture(t)
	for _, rt := range routes(nobody, f.svc, f.db) {
		code, _ := f.call(t, nobody, rt.method, rt.path, uuid.NewString(), map[string]any{}, f.org)
		assert.Equal(t, http.StatusForbidden, code, "%s %s", rt.method, rt.path)
	}

	code, data := f.call(t, manager, "GET", base+"/settings", "", nil, f.org)
	require.Equal(t, 200, code)
	assert.Equal(t, false, data["provider"].(map[string]any)["key_set"])
	code, _ = f.call(t, manager, "PUT", base+"/settings", "", map[string]any{"enabled": true, "emergency_words": []string{"chest pain"}}, f.org)
	require.Equal(t, 200, code)

	code, data = f.call(t, manager, "POST", base+"/knowledge", "", map[string]any{"kind": "faq", "question": "Parking?", "answer": "Bay Lane."}, f.org)
	require.Equal(t, 200, code)
	src := data["item"].(map[string]any)["source_id"].(string)
	code, data = f.call(t, manager, "GET", base+"/knowledge", "", nil, f.org)
	require.Equal(t, 200, code)
	assert.Len(t, data["items"], 1)

	f.llm.intents["CHEST PAIN"] = `{"intent":"clinical","confidence":1}`
	code, _ = f.call(t, staff, "POST", base+"/reply", "", map[string]any{"contact_id": f.contact, "text": "chest pain"}, f.org)
	assert.Equal(t, http.StatusForbidden, code, "reply also needs settings read")
	code, data = f.call(t, manager, "POST", base+"/reply", "", map[string]any{"contact_id": f.contact, "text": "chest pain", "send": true}, f.org)
	require.Equal(t, 200, code)
	assert.Equal(t, OutcomeEmergency, data["outcome"])
	assert.Equal(t, true, data["handed_over"])
	assert.Len(t, f.sender.sent, 1)

	code, data = f.call(t, staff, "GET", base+"/runs", "", nil, f.org)
	require.Equal(t, 200, code)
	runs := data["runs"].([]any)
	require.Len(t, runs, 1)
	assert.Regexp(t, `^61•+678$`, runs[0].(map[string]any)["contact"].(map[string]any)["phone_masked"])

	other := testutil.CreateTestOrganization(t, f.db)
	code, data = f.call(t, manager, "GET", base+"/knowledge", "", nil, other.ID)
	require.Equal(t, 200, code)
	assert.Empty(t, data["items"], "another organization sees none of it")
	code, _ = f.call(t, manager, "DELETE", base+"/knowledge/{id}", src, nil, other.ID)
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = f.call(t, manager, "POST", base+"/reply", "", map[string]any{"contact_id": f.contact, "text": "hi"}, other.ID)
	assert.Equal(t, http.StatusNotFound, code, "another organization's contact")
	code, _ = f.call(t, manager, "DELETE", base+"/knowledge/{id}", src, nil, f.org)
	assert.Equal(t, 200, code)
}
