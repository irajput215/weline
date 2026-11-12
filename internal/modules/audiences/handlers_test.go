package audiences

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerodha/fastglue"
)

// fakeAuth plays *handlers.App: active org from the request context,
// permissions from a fixed set ("resource:action").
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
	manager = perms(PermSegments+":read", PermSegments+":create", PermSegments+":update", PermSegments+":delete",
		PermSends+":create", "campaigns:write")
	reception    = perms(PermSegments + ":read")
	noCampaigns  = perms(PermSegments+":read", PermSegments+":create", PermSends+":create")
	nobody       = perms()
	segmentsPath = base + "/segments"
	segmentPath  = base + "/segments/{id}"
)

type req struct {
	method, path string
	id           string
	query        map[string]string
	body         any
}

type resp struct {
	code int
	data map[string]any
	msg  string
}

func run(t *testing.T, auth kit.Auth, svc *Service, r req, org, user uuid.UUID) resp {
	t.Helper()
	var h fastglue.FastRequestHandler
	for _, rt := range routes(auth, svc) {
		if rt.method == r.method && rt.path == r.path {
			h = rt.handler
		}
	}
	require.NotNil(t, h, "no route %s %s", r.method, r.path)
	rq := testutil.NewRequest(t)
	rq.RequestCtx.Request.Header.SetMethod(r.method)
	if r.body != nil {
		b, err := json.Marshal(r.body)
		require.NoError(t, err)
		rq.RequestCtx.Request.SetBody(b)
	}
	if r.id != "" {
		rq.RequestCtx.SetUserValue("id", r.id)
	}
	for k, v := range r.query {
		rq.RequestCtx.QueryArgs().Set(k, v)
	}
	if org != uuid.Nil {
		testutil.SetAuthContext(rq, org, user)
	}
	require.NoError(t, h(rq))
	var env struct {
		Data    map[string]any `json:"data"`
		Message string         `json:"message"`
	}
	_ = json.Unmarshal(rq.RequestCtx.Response.Body(), &env)
	return resp{code: rq.RequestCtx.Response.StatusCode(), data: env.Data, msg: env.Message}
}

func (f *fixture) api(t *testing.T, auth kit.Auth, r req) resp {
	t.Helper()
	return run(t, auth, f.svc, r, f.org, f.user)
}

func TestHandlers_EveryRouteChecksPermission(t *testing.T) {
	f := newFixture(t)
	for _, rt := range routes(nobody, f.svc) {
		r := run(t, nobody, f.svc, req{method: rt.method, path: rt.path, id: uuid.NewString(), body: map[string]any{}}, f.org, f.user)
		assert.Equal(t, http.StatusForbidden, r.code, "%s %s without permission", rt.method, rt.path)
		r = run(t, manager, f.svc, req{method: rt.method, path: rt.path, id: uuid.NewString()}, uuid.Nil, uuid.Nil)
		assert.Equal(t, http.StatusUnauthorized, r.code, "%s %s without a user", rt.method, rt.path)
	}
}

func TestHandlers_SegmentsFlow(t *testing.T) {
	f := newFixture(t)
	f.contact(t, f.org, tags("flu"), named("Alice"))
	f.contact(t, f.org, tags("flu"))

	body := map[string]any{"name": "Flu", "rules": rulesOf(t, "all", tagRule("flu"))}
	r := f.api(t, reception, req{method: "POST", path: segmentsPath, body: body})
	assert.Equal(t, http.StatusForbidden, r.code, "reception can't create segments")

	r = f.api(t, manager, req{method: "POST", path: segmentsPath, body: body})
	require.Equal(t, http.StatusOK, r.code, r.msg)
	seg, _ := r.data["segment"].(map[string]any)
	segID, _ := seg["id"].(string)
	require.NotEmpty(t, segID)

	r = f.api(t, manager, req{method: "POST", path: segmentsPath, body: body})
	assert.Equal(t, http.StatusConflict, r.code)
	r = f.api(t, manager, req{method: "POST", path: segmentsPath,
		body: map[string]any{"name": "Bad", "rules": rulesOf(t, "all", map[string]any{"field": "metadata", "op": "eq", "key": "x'--", "value": "1"})}})
	assert.Equal(t, http.StatusBadRequest, r.code)
	r = f.api(t, manager, req{method: "POST", path: segmentsPath, body: "not an object"})
	assert.Equal(t, http.StatusBadRequest, r.code)

	r = f.api(t, reception, req{method: "GET", path: segmentsPath})
	require.Equal(t, http.StatusOK, r.code)
	list, _ := r.data["segments"].([]any)
	require.Len(t, list, 1)
	assert.EqualValues(t, 2, list[0].(map[string]any)["count"])

	r = f.api(t, reception, req{method: "POST", path: base + "/preview", body: map[string]any{"rules": rulesOf(t, "all", tagRule("flu"))}})
	require.Equal(t, http.StatusOK, r.code)
	assert.EqualValues(t, 2, r.data["recipients"])
	sample, _ := r.data["sample"].([]any)
	require.Len(t, sample, 2)
	assert.NotContains(t, sample[0].(map[string]any)["phone_masked"], "+")

	r = f.api(t, manager, req{method: "PUT", path: segmentPath, id: segID, body: map[string]any{"description": "Winter"}})
	require.Equal(t, http.StatusOK, r.code, r.msg)
	assert.Equal(t, "Winter", r.data["segment"].(map[string]any)["description"])

	r = f.api(t, manager, req{method: "GET", path: segmentPath, id: "nope"})
	assert.Equal(t, http.StatusNotFound, r.code)

	r = f.api(t, reception, req{method: "GET", path: base + "/fields"})
	require.Equal(t, http.StatusOK, r.code)
	assert.Len(t, r.data["templates"], 2)

	r = f.api(t, manager, req{method: "DELETE", path: segmentPath, id: segID})
	require.Equal(t, http.StatusOK, r.code)
	assert.Equal(t, true, r.data["deleted"])
}

func TestHandlers_OrgIsolation(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)
	other.contact(t, other.org, tags("flu"))
	seg, err := other.svc.CreateSegment(ctx, other.org, other.user, SegmentInput{Name: "Theirs", Rules: rulesOf(t, "all", tagRule("flu"))})
	require.NoError(t, err)
	camp := f.campaign(t, other)

	id := seg.ID.String()
	assert.Equal(t, http.StatusNotFound, f.api(t, manager, req{method: "GET", path: segmentPath, id: id}).code)
	assert.Equal(t, http.StatusNotFound, f.api(t, manager, req{method: "PUT", path: segmentPath, id: id, body: map[string]any{"name": "Mine"}}).code)
	assert.Equal(t, http.StatusNotFound, f.api(t, manager, req{method: "DELETE", path: segmentPath, id: id}).code)

	r := f.api(t, manager, req{method: "GET", path: segmentsPath})
	assert.Empty(t, r.data["segments"])
	r = f.api(t, manager, req{method: "POST", path: base + "/preview", body: map[string]any{"rules": rulesOf(t, "all", tagRule("flu"))}})
	assert.EqualValues(t, 0, r.data["matched"], "other organizations' contacts never match")
	r = f.api(t, manager, req{method: "POST", path: base + "/preview",
		body: map[string]any{"retarget": map[string]any{"campaign_id": camp.ID, "filter": "not_read"}}})
	assert.Equal(t, http.StatusNotFound, r.code, "another organization's campaign")

	r = f.api(t, manager, req{method: "POST", path: base + "/sends", body: map[string]any{"name": "x", "whatsapp_account": f.account,
		"template_id": f.util.ID, "segment_id": seg.ID, "confirm_count": 1}})
	assert.Equal(t, http.StatusNotFound, r.code, "another organization's segment")

	r = f.api(t, manager, req{method: "GET", path: base + "/fields"})
	assert.Empty(t, r.data["campaigns"])
}

func TestHandlers_Sends(t *testing.T) {
	f := newFixture(t)
	f.contact(t, f.org, tags("vip"))
	seg, err := f.svc.CreateSegment(ctx, f.org, f.user, SegmentInput{Name: "VIP", Rules: rulesOf(t, "all", tagRule("vip"))})
	require.NoError(t, err)
	body := map[string]any{"name": "VIP news", "whatsapp_account": f.account, "template_id": f.util.ID,
		"segment_id": seg.ID, "variables": map[string]any{"body": map[string]any{}}, "confirm_count": 1}

	r := f.api(t, noCampaigns, req{method: "POST", path: base + "/sends", body: body})
	assert.Equal(t, http.StatusForbidden, r.code, "needs Weline's campaigns:write too")

	body["confirm_count"] = 5
	r = f.api(t, manager, req{method: "POST", path: base + "/sends", body: body})
	assert.Equal(t, http.StatusConflict, r.code, "audience changed")

	body["confirm_count"] = 1
	r = f.api(t, manager, req{method: "POST", path: base + "/sends", body: body})
	require.Equal(t, http.StatusOK, r.code, r.msg)
	campID, _ := r.data["campaign_id"].(string)
	var camp models.BulkMessageCampaign
	require.NoError(t, f.db.Where("organization_id = ? AND id = ?", f.org, campID).First(&camp).Error)
	assert.Equal(t, models.CampaignStatusDraft, camp.Status)

	r = f.api(t, reception, req{method: "GET", path: base + "/sends", query: map[string]string{"segment_id": seg.ID.String()}})
	require.Equal(t, http.StatusOK, r.code)
	assert.Len(t, r.data["sends"], 1)
	r = f.api(t, reception, req{method: "GET", path: base + "/sends", query: map[string]string{"campaign_id": "bad"}})
	assert.Equal(t, http.StatusBadRequest, r.code)
}
