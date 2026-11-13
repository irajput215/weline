package packs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/ai"
	"github.com/shridarpatil/whatomate/internal/modules/audiences"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/reminders"
	"gorm.io/gorm"
)

// Item statuses in an install result.
const (
	StatusCreated       = "created"          // made now
	StatusExists        = "exists"           // already there (left alone)
	StatusKept          = "kept"             // you had your own; not overwritten
	StatusWaiting       = "waiting_approval" // its template isn't approved by Meta yet; install again later
	StatusNeedsWhatsApp = "needs_whatsapp"   // connect a WhatsApp number first
	StatusFailed        = "failed"
)

// Item is one thing an install did.
type Item struct {
	Kind    string `json:"kind"` // template | ai | segment | reminder
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Result is what an install did.
type Result struct {
	PackID      string    `json:"pack_id"`
	Version     int       `json:"version"`
	Items       []Item    `json:"items"`
	InstalledAt time.Time `json:"installed_at"`
}

// Service installs packs using the other modules' services, so their rules
// (template approval, segment validation, AI settings limits) still apply.
type Service struct {
	db        *gorm.DB
	ai        *ai.Service
	audiences *audiences.Service
	reminders *reminders.Service
	now       func() time.Time
}

// NewService builds the service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db, ai: ai.NewService(db, nil, nil, nil, nil, nil, nil), audiences: audiences.NewService(db),
		reminders: reminders.NewService(db, nil, nil, nil, nil, nil), now: time.Now}
}

// Summary is a pack in the list, with this organization's last install.
type Summary struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     int            `json:"version"`
	Recommended bool           `json:"recommended"` // fits the vertical chosen in onboarding
	Installed   *Result        `json:"installed"`
	Counts      map[string]int `json:"counts"`
}

func (s *Service) lastInstall(ctx context.Context, orgID uuid.UUID, packID string) (*Result, error) {
	var rows []struct {
		Result json.RawMessage
	}
	if err := s.db.WithContext(ctx).Raw(`SELECT result FROM wl.packs_installs WHERE organization_id = ? AND pack_id = ?
		ORDER BY installed_at DESC LIMIT 1`, orgID, packID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	var r Result
	if err := json.Unmarshal(rows[0].Result, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) vertical(ctx context.Context, orgID uuid.UUID) string {
	var v struct{ Vertical string }
	_ = s.db.WithContext(ctx).Raw(`SELECT vertical FROM wl.onboarding_state WHERE organization_id = ?`, orgID).Scan(&v).Error
	return v.Vertical
}

// List returns every pack, the recommended one first.
func (s *Service) List(ctx context.Context, orgID uuid.UUID) ([]Summary, error) {
	rec := ForVertical(s.vertical(ctx, orgID))
	var out []Summary
	for _, p := range All() {
		last, err := s.lastInstall(ctx, orgID, p.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, Summary{ID: p.ID, Name: p.Name, Description: p.Description, Version: p.Version, Recommended: p.ID == rec,
			Installed: last, Counts: map[string]int{"templates": len(p.Templates), "segments": len(p.Segments), "reminders": len(p.Reminders), "flows": len(p.Flows)}})
	}
	for i := range out {
		if out[i].Recommended && i > 0 {
			out[0], out[i] = out[i], out[0]
		}
	}
	return out, nil
}

// Get returns a pack's full contents to preview.
func (s *Service) Get(id string) (*Pack, error) {
	p, ok := byID(id)
	if !ok {
		return nil, kit.NotFound("pack not found")
	}
	return &p, nil
}

func (s *Service) account(ctx context.Context, orgID uuid.UUID) (string, error) {
	var names []string
	err := s.db.WithContext(ctx).Model(&models.WhatsAppAccount{}).Where("organization_id = ?", orgID).
		Order("is_default_outgoing DESC, created_at").Limit(1).Pluck("name", &names).Error
	if err != nil || len(names) == 0 {
		return "", err
	}
	return names[0], nil
}

// Install adds the pack's items to the organization. Items already there are
// left alone, so installing again is safe and finishes what waited.
func (s *Service) Install(ctx context.Context, orgID, userID uuid.UUID, id string) (*Result, error) {
	p, ok := byID(id)
	if !ok {
		return nil, kit.NotFound("pack not found")
	}
	res := &Result{PackID: p.ID, Version: p.Version, InstalledAt: s.now().UTC()}
	add := func(kind, name, status, msg string) {
		res.Items = append(res.Items, Item{Kind: kind, Name: name, Status: status, Message: msg})
	}

	// Templates: local drafts on the WhatsApp number, to submit to Meta.
	acc, err := s.account(ctx, orgID)
	if err != nil {
		return nil, err
	}
	approved := map[string]bool{}
	for _, t := range p.Templates {
		var existing []models.Template
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND name = ? AND language = ?", orgID, t.Name, t.Language).
			Limit(1).Find(&existing).Error; err != nil {
			return nil, err
		}
		switch {
		case len(existing) == 1:
			approved[t.Name] = existing[0].Status == "APPROVED"
			add("template", t.Name, StatusExists, strings.ToLower(existing[0].Status))
		case acc == "":
			add("template", t.Name, StatusNeedsWhatsApp, "")
		default:
			if err := s.db.WithContext(ctx).Create(templateRow(orgID, userID, acc, t)).Error; err != nil {
				return nil, fmt.Errorf("create template: %w", err)
			}
			add("template", t.Name, StatusCreated, "draft")
		}
	}

	// AI agent: tone and emergency words added; instructions only if you have none.
	if err := s.applyAI(ctx, orgID, userID, p.AI, add); err != nil {
		return nil, err
	}

	// Segments, by name.
	segs, err := s.audiences.ListSegments(ctx, orgID)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, sg := range segs {
		have[strings.ToLower(sg.Name)] = true
	}
	for _, sg := range p.Segments {
		if have[strings.ToLower(sg.Name)] {
			add("segment", sg.Name, StatusExists, "")
			continue
		}
		var rules models.JSONB
		if err := json.Unmarshal(sg.Rules, &rules); err != nil {
			return nil, err
		}
		if _, err := s.audiences.CreateSegment(ctx, orgID, userID, audiences.SegmentInput{Name: sg.Name, Description: sg.Description, Rules: rules}); err != nil {
			add("segment", sg.Name, StatusFailed, safeMessage(err))
			continue
		}
		add("segment", sg.Name, StatusCreated, "")
	}

	// Reminder and recall rules, once their template is approved.
	rules, err := s.reminders.ListRules(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, r := range p.Reminders {
		exists := false
		for _, x := range rules {
			exists = exists || (x.Kind == r.Kind && strings.EqualFold(x.Name, r.Name))
		}
		tpl := templateOf(p, r.Template)
		switch {
		case exists:
			add("reminder", r.Name, StatusExists, "")
		case !approved[r.Template]:
			add("reminder", r.Name, StatusWaiting, r.Template)
		default:
			_, err := s.reminders.SaveRule(ctx, orgID, userID, nil, reminders.Rule{Kind: r.Kind, Name: r.Name, OffsetMinutes: r.OffsetMinutes,
				RecallDays: r.RecallDays, TemplateName: r.Template, Language: tpl.Language, Variables: r.Variables, Enabled: false})
			if err != nil {
				add("reminder", r.Name, StatusFailed, safeMessage(err))
				continue
			}
			add("reminder", r.Name, StatusCreated, "off until you turn it on")
		}
	}

	b, _ := json.Marshal(res)
	if err := s.db.WithContext(ctx).Exec(`INSERT INTO wl.packs_installs (organization_id, pack_id, pack_version, result, installed_by_id)
		VALUES (?, ?, ?, ?, ?)`, orgID, p.ID, p.Version, b, userID).Error; err != nil {
		return nil, err
	}
	created := 0
	for _, it := range res.Items {
		if it.Status == StatusCreated {
			created++
		}
	}
	audit.Log(ctx, s.db, nil, orgID, audit.Entry{Actor: &userID, Module: Name, Action: "pack_installed", Entity: "pack", EntityID: p.ID,
		Details: map[string]any{"version": p.Version, "created": created}})
	return res, nil
}

func templateOf(p Pack, name string) Template {
	for _, t := range p.Templates {
		if t.Name == name {
			return t
		}
	}
	return Template{}
}

func templateRow(orgID, userID uuid.UUID, account string, t Template) *models.Template {
	buttons := models.JSONBArray{}
	for _, b := range t.Buttons {
		buttons = append(buttons, map[string]any{"type": "QUICK_REPLY", "text": b})
	}
	samples := models.JSONBArray{}
	for i, v := range t.Samples {
		samples = append(samples, map[string]any{"component": "body", "index": i + 1, "value": v})
	}
	return &models.Template{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: orgID, WhatsAppAccount: account, Name: t.Name,
		DisplayName: t.Name, Language: t.Language, Category: t.Category, Status: "DRAFT", BodyContent: t.Body, FooterContent: t.Footer,
		Buttons: buttons, SampleValues: samples, CreatedByID: &userID, UpdatedByID: &userID}
}

func (s *Service) applyAI(ctx context.Context, orgID, userID uuid.UUID, a AI, add func(kind, name, status, msg string)) error {
	st, err := s.ai.GetSettings(ctx, orgID)
	if err != nil {
		return err
	}
	changed := false
	switch {
	case strings.TrimSpace(a.Instructions) == "":
	case strings.TrimSpace(st.Instructions) == "":
		st.Instructions, changed = a.Instructions, true
		if a.Tone != "" { // the pack's voice comes with its instructions
			st.Tone = a.Tone
		}
		add("ai", "instructions", StatusCreated, "")
	case strings.TrimSpace(st.Instructions) == strings.TrimSpace(a.Instructions):
		add("ai", "instructions", StatusExists, "")
	default:
		add("ai", "instructions", StatusKept, "")
	}
	seen := map[string]bool{}
	for _, w := range st.EmergencyWords {
		seen[strings.ToLower(strings.TrimSpace(w))] = true
	}
	added := 0
	for _, w := range a.EmergencyWords {
		if k := strings.ToLower(strings.TrimSpace(w)); k != "" && !seen[k] {
			st.EmergencyWords, seen[k] = append(st.EmergencyWords, w), true
			added++
		}
	}
	if added > 0 {
		changed = true
		add("ai", "emergency words", StatusCreated, fmt.Sprintf("%d added", added))
	} else if len(a.EmergencyWords) > 0 {
		add("ai", "emergency words", StatusExists, "")
	}
	if !changed {
		return nil
	}
	_, err = s.ai.SaveSettings(ctx, orgID, userID, st)
	return err
}

// safeMessage keeps a module's own message for known errors, nothing else.
func safeMessage(err error) string {
	var ke *kit.Error
	if errors.As(err, &ke) {
		return ke.Message
	}
	return "couldn't be added"
}
