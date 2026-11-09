package practice

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

type route struct {
	method, path string
	handler      fastglue.FastRequestHandler
}

const base = "/api/x/practice"

// routes wires every endpoint (api.md) to its permission. Tests call it with
// a fake auth so they exercise the same wiring as production.
func routes(auth kit.Auth, svc *Service) []route {
	h := &handler{svc: svc}
	g := func(res, act string, fn func(*fastglue.Request, kit.Caller) error) fastglue.FastRequestHandler {
		return kit.Guard(auth, res, act, fn)
	}
	return []route{
		{"GET", base + "/profile", g(PermSettings, kit.ActionRead, h.getProfile)},
		{"PUT", base + "/profile", g(PermSettings, kit.ActionUpdate, h.saveProfile)},

		{"GET", base + "/locations", g(PermLocations, kit.ActionRead, h.listLocations)},
		{"POST", base + "/locations", g(PermLocations, kit.ActionCreate, h.createLocation)},
		{"PUT", base + "/locations/{id}", g(PermLocations, kit.ActionUpdate, h.updateLocation)},

		{"GET", base + "/practitioners", g(PermPractitioners, kit.ActionRead, h.listPractitioners)},
		{"POST", base + "/practitioners", g(PermPractitioners, kit.ActionCreate, h.createPractitioner)},
		{"PUT", base + "/practitioners/{id}", g(PermPractitioners, kit.ActionUpdate, h.updatePractitioner)},
		{"GET", base + "/members", g(PermPractitioners, kit.ActionRead, h.listMembers)},

		{"GET", base + "/services", g(PermServices, kit.ActionRead, h.listServices)},
		{"POST", base + "/services", g(PermServices, kit.ActionCreate, h.createService)},
		{"PUT", base + "/services/{id}", g(PermServices, kit.ActionUpdate, h.updateService)},

		{"GET", base + "/locations/{id}/hours", g(PermSettings, kit.ActionRead, h.getHours)},
		{"PUT", base + "/locations/{id}/hours", g(PermSettings, kit.ActionUpdate, h.setHours)},
		{"GET", base + "/closures", g(PermSettings, kit.ActionRead, h.listClosures)},
		{"POST", base + "/closures", g(PermSettings, kit.ActionUpdate, h.createClosure)},
		{"DELETE", base + "/closures/{id}", g(PermSettings, kit.ActionUpdate, h.deleteClosure)},
	}
}

// handler is the thin HTTP layer: read the request, call the service, reply.
type handler struct {
	svc *Service
}

func decode(r *fastglue.Request, v any) error {
	if err := json.Unmarshal(r.RequestCtx.PostBody(), v); err != nil {
		return kit.Invalid("invalid request body")
	}
	return nil
}

func pathID(r *fastglue.Request, what string) (uuid.UUID, error) {
	s, _ := r.RequestCtx.UserValue("id").(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, kit.NotFound("%s not found", what)
	}
	return id, nil
}

func includeInactive(r *fastglue.Request) bool {
	return string(r.RequestCtx.QueryArgs().Peek("include_inactive")) == "true"
}

func ok(r *fastglue.Request, data any) error { return r.SendEnvelope(data) }

// ---------- profile ----------

func (h *handler) getProfile(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	missing, err := h.svc.SetupMissing(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	p, err := h.svc.GetProfile(ctx, c.OrgID)
	if IsNotSetUp(err) {
		return ok(r, map[string]any{"profile": nil, "setup": map[string]any{"missing": missing}, "time_zones": TimeZones, "slot_lengths": SlotLengths})
	}
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"profile": p, "setup": map[string]any{"missing": missing}, "time_zones": TimeZones, "slot_lengths": SlotLengths})
}

func (h *handler) saveProfile(r *fastglue.Request, c kit.Caller) error {
	var in ProfileInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	p, err := h.svc.SaveProfile(ctx, c.OrgID, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"profile": p})
}

// ---------- locations ----------

func (h *handler) listLocations(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	list, err := h.svc.ListLocations(ctx, c.OrgID, includeInactive(r))
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"locations": list})
}

func (h *handler) createLocation(r *fastglue.Request, c kit.Caller) error {
	var in LocationInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	l, err := h.svc.CreateLocation(ctx, c.OrgID, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"location": l})
}

func (h *handler) updateLocation(r *fastglue.Request, c kit.Caller) error {
	id, err := pathID(r, "location")
	if err != nil {
		return kit.SendError(r, err)
	}
	var in LocationPatch
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	l, err := h.svc.UpdateLocation(ctx, c.OrgID, id, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"location": l})
}

// ---------- practitioners ----------

func (h *handler) listPractitioners(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	list, err := h.svc.ListPractitioners(ctx, c.OrgID, includeInactive(r))
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"practitioners": list})
}

func (h *handler) createPractitioner(r *fastglue.Request, c kit.Caller) error {
	var in PractitionerInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	p, err := h.svc.CreatePractitioner(ctx, c.OrgID, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"practitioner": p})
}

func (h *handler) updatePractitioner(r *fastglue.Request, c kit.Caller) error {
	id, err := pathID(r, "practitioner")
	if err != nil {
		return kit.SendError(r, err)
	}
	var in PractitionerPatch
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	p, err := h.svc.UpdatePractitioner(ctx, c.OrgID, id, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"practitioner": p})
}

func (h *handler) listMembers(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	list, err := h.svc.ListMembers(ctx, c.OrgID)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"members": list})
}

// ---------- services ----------

func (h *handler) listServices(r *fastglue.Request, c kit.Caller) error {
	ctx, cancel := kit.Context()
	defer cancel()
	list, err := h.svc.ListServices(ctx, c.OrgID, includeInactive(r))
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"services": list})
}

func (h *handler) createService(r *fastglue.Request, c kit.Caller) error {
	var in ServiceInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	sv, warnings, err := h.svc.CreateService(ctx, c.OrgID, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"service": sv, "warnings": warnings})
}

func (h *handler) updateService(r *fastglue.Request, c kit.Caller) error {
	id, err := pathID(r, "service")
	if err != nil {
		return kit.SendError(r, err)
	}
	var in ServicePatch
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	sv, warnings, err := h.svc.UpdateService(ctx, c.OrgID, id, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"service": sv, "warnings": warnings})
}

// ---------- hours and closures ----------

func (h *handler) getHours(r *fastglue.Request, c kit.Caller) error {
	id, err := pathID(r, "location")
	if err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	hours, err := h.svc.GetHours(ctx, c.OrgID, id)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"location_id": id, "hours": hours})
}

func (h *handler) setHours(r *fastglue.Request, c kit.Caller) error {
	id, err := pathID(r, "location")
	if err != nil {
		return kit.SendError(r, err)
	}
	var in struct {
		Hours []Hours `json:"hours"`
	}
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	hours, err := h.svc.SetHours(ctx, c.OrgID, id, in.Hours)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"location_id": id, "hours": hours})
}

func (h *handler) listClosures(r *fastglue.Request, c kit.Caller) error {
	from := Date{time.Now().UTC().Truncate(24 * time.Hour)}
	if s := string(r.RequestCtx.QueryArgs().Peek("from")); s != "" {
		d, err := ParseDate(s)
		if err != nil {
			return kit.SendError(r, kit.Invalid("from must be YYYY-MM-DD"))
		}
		from = d
	}
	ctx, cancel := kit.Context()
	defer cancel()
	list, err := h.svc.ListClosures(ctx, c.OrgID, from)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"closures": list})
}

func (h *handler) createClosure(r *fastglue.Request, c kit.Caller) error {
	var in ClosureInput
	if err := decode(r, &in); err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	cl, err := h.svc.CreateClosure(ctx, c.OrgID, in)
	if err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"closure": cl})
}

func (h *handler) deleteClosure(r *fastglue.Request, c kit.Caller) error {
	id, err := pathID(r, "closure")
	if err != nil {
		return kit.SendError(r, err)
	}
	ctx, cancel := kit.Context()
	defer cancel()
	if err := h.svc.DeleteClosure(ctx, c.OrgID, id); err != nil {
		return kit.SendError(r, err)
	}
	return ok(r, map[string]any{"deleted": true})
}
