// Package audit records who changed module settings and connections, and when
// (step 3.3, threat model open risk 7): connected system credentials and
// tests, clinic setup, consent settings, ads entry points. Booking and Consent
// keep their own per-record histories; this is the cross-module trail.
// Entries never hold patient names, numbers or message text, or secrets.
// Spec: wl-docs/modules/audit.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// Name is the module name used for migrations and logs.
const Name = "audit"

// PermEvents is the permission to read the trail.
const PermEvents = "wl_audit_events"

// Entry is one change.
type Entry struct {
	Actor    *uuid.UUID     // the user who made it; nil for system jobs
	Module   string         // "connectors", "practice", …
	Action   string         // "credentials_set", "hours_saved", …
	Entity   string         // "connection", "location", …
	EntityID string         // the record's id, if any
	Details  map[string]any // what changed, without personal data or secrets
}

type row struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID  `gorm:"type:uuid"`
	ActorUserID    *uuid.UUID `gorm:"type:uuid"`
	Module         string
	Action         string
	Entity         string
	EntityID       string
	Details        json.RawMessage `gorm:"type:jsonb"`
	At             time.Time
}

func (row) TableName() string { return "wl.audit_events" }

// blocked keys are dropped from details, as a last guard against personal
// data or secrets reaching the trail.
var blocked = []string{"phone", "mobile", "name", "email", "key", "secret", "token", "password", "credential", "text", "content", "note"}

func clean(d map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range d {
		lk := strings.ToLower(k)
		skip := false
		for _, b := range blocked {
			if strings.Contains(lk, b) && !strings.HasSuffix(lk, "_set") && !strings.HasSuffix(lk, "_count") {
				skip = true
				break
			}
		}
		if !skip {
			out[k] = v
		}
	}
	return out
}

// Record adds an entry. Callers log a failure and carry on: a missing audit
// row must not undo the change it describes.
func Record(ctx context.Context, db *gorm.DB, orgID uuid.UUID, e Entry) error {
	if orgID == uuid.Nil || e.Module == "" || e.Action == "" {
		return kit.Invalid("audit: organization, module and action are required")
	}
	b, err := json.Marshal(clean(e.Details))
	if err != nil {
		return fmt.Errorf("audit: encode details: %w", err)
	}
	r := row{OrganizationID: orgID, ActorUserID: e.Actor, Module: e.Module, Action: e.Action, Entity: e.Entity,
		EntityID: e.EntityID, Details: b, At: time.Now().UTC()}
	if err := db.WithContext(ctx).Create(&r).Error; err != nil {
		return fmt.Errorf("audit: record: %w", err)
	}
	return nil
}

// Logger is what Log needs.
type Logger interface {
	Error(msg string, fields ...any)
}

// Log records an entry and logs (without details) when that fails.
func Log(ctx context.Context, db *gorm.DB, log Logger, orgID uuid.UUID, e Entry) {
	if err := Record(ctx, db, orgID, e); err != nil && log != nil {
		log.Error("audit entry not recorded", "organization_id", orgID, "module", e.Module, "action", e.Action, "error", err)
	}
}

// Event is an entry as the API shows it.
type Event struct {
	ID       uuid.UUID       `json:"id"`
	Actor    *ActorRef       `json:"actor"`
	Module   string          `json:"module"`
	Action   string          `json:"action"`
	Entity   string          `json:"entity"`
	EntityID string          `json:"entity_id"`
	Details  json.RawMessage `json:"details"`
	At       time.Time       `json:"at"`
}

// ActorRef names the user who made a change.
type ActorRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// List returns the organization's trail, newest first, 50 at a time.
func List(ctx context.Context, db *gorm.DB, orgID uuid.UUID, module string, before *time.Time) ([]Event, error) {
	q := db.WithContext(ctx).Table("wl.audit_events a").
		Select("a.*, u.full_name AS actor_name").
		Joins(`LEFT JOIN users u ON u.id = a.actor_user_id AND (u.organization_id = a.organization_id OR EXISTS
			(SELECT 1 FROM user_organizations uo WHERE uo.user_id = u.id AND uo.organization_id = a.organization_id AND uo.deleted_at IS NULL))`).
		Where("a.organization_id = ?", orgID)
	if module != "" {
		q = q.Where("a.module = ?", module)
	}
	if before != nil {
		q = q.Where("a.at < ?", *before)
	}
	// Flat on purpose: GORM's Scan doesn't fill an embedded struct from a raw select.
	var rows []struct {
		ID          uuid.UUID
		ActorUserID *uuid.UUID
		Module      string
		Action      string
		Entity      string
		EntityID    string
		Details     json.RawMessage
		At          time.Time
		ActorName   *string
	}
	if err := q.Order("a.at DESC, a.id").Limit(50).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		ev := Event{ID: r.ID, Module: r.Module, Action: r.Action, Entity: r.Entity, EntityID: r.EntityID, Details: r.Details, At: r.At}
		if r.ActorUserID != nil {
			name := ""
			if r.ActorName != nil {
				name = *r.ActorName
			}
			ev.Actor = &ActorRef{ID: *r.ActorUserID, Name: name}
		}
		out = append(out, ev)
	}
	return out, nil
}

// Wrap records an entry after a write handler succeeds (status 200), with
// the record id from the route's {id}, so modules audit routes without
// changing each handler. Details stay empty: the action and record say what
// changed, and request bodies may hold personal data.
func Wrap(db *gorm.DB, log Logger, module, action, entity string,
	fn func(*fastglue.Request, kit.Caller) error) func(*fastglue.Request, kit.Caller) error {
	return func(r *fastglue.Request, c kit.Caller) error {
		err := fn(r, c)
		if err != nil || r.RequestCtx.Response.StatusCode() != fasthttp.StatusOK {
			return err
		}
		id, _ := r.RequestCtx.UserValue("id").(string)
		actor := c.UserID
		ctx, cancel := kit.Context()
		defer cancel()
		Log(ctx, db, log, c.OrgID, Entry{Actor: &actor, Module: module, Action: action, Entity: entity, EntityID: id})
		return nil
	}
}
