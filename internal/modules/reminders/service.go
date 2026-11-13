package reminders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/consent"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Kinds of rule.
const (
	KindReminder = "reminder"
	KindRecall   = "recall"

	sweepEvery = 5 * time.Minute
	sweepLock  = "wl:reminders:sweep"
)

// Variables a rule can fill, in the template's order {{1}}, {{2}}, ...
var variables = map[string]bool{"name": true, "service": true, "when": true, "location": true, "practitioner": true}

// Handover opens an agent transfer (Weline's accessor).
type Handover interface {
	HandOver(orgID, contactID uuid.UUID, teamID *uuid.UUID, notes string) (bool, error)
}

// Logger is the part of Weline's logger the module uses.
type Logger interface {
	Error(msg string, fields ...any)
}

type nopLogger struct{}

func (nopLogger) Error(string, ...any) {}

// Service holds the reminder rules; calls take the organization first.
type Service struct {
	db        *gorm.DB
	rdb       *redis.Client
	providers provider.Resolver
	sender    kit.Sender
	consent   *consent.Service
	handover  Handover
	log       Logger
	now       func() time.Time
}

// NewService returns the reminders service.
func NewService(db *gorm.DB, rdb *redis.Client, providers provider.Resolver, sender kit.Sender, handover Handover, log Logger) *Service {
	if log == nil {
		log = nopLogger{}
	}
	return &Service{db: db, rdb: rdb, providers: providers, sender: sender, consent: consent.NewService(db, nil, nil, nil),
		handover: handover, log: log, now: time.Now}
}

// ---------- rules (R1, R7) ----------

// Rule is a reminder or recall rule.
type Rule struct {
	ID            uuid.UUID `json:"id"`
	Kind          string    `json:"kind"`
	Name          string    `json:"name"`
	OffsetMinutes int       `json:"offset_minutes"`
	RecallDays    int       `json:"recall_days"`
	TemplateName  string    `json:"template_name"`
	Language      string    `json:"language"`
	Variables     []string  `json:"variables"`
	ServiceIDs    []string  `json:"service_ids"`
	Enabled       bool      `json:"enabled"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ruleRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	Kind           string
	Name           string
	OffsetMinutes  int
	RecallDays     int
	TemplateName   string
	Language       string
	Variables      json.RawMessage `gorm:"type:jsonb"`
	ServiceIDs     json.RawMessage `gorm:"type:jsonb;column:service_ids"`
	Enabled        bool
	CreatedByID    *uuid.UUID `gorm:"type:uuid"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (ruleRow) TableName() string { return "wl.reminder_rules" }

func (r ruleRow) view() Rule {
	v := Rule{ID: r.ID, Kind: r.Kind, Name: r.Name, OffsetMinutes: r.OffsetMinutes, RecallDays: r.RecallDays, TemplateName: r.TemplateName,
		Language: r.Language, Enabled: r.Enabled, UpdatedAt: r.UpdatedAt, Variables: []string{}, ServiceIDs: []string{}}
	_ = json.Unmarshal(r.Variables, &v.Variables)
	_ = json.Unmarshal(r.ServiceIDs, &v.ServiceIDs)
	return v
}

func (r Rule) appliesTo(serviceID string) bool {
	if len(r.ServiceIDs) == 0 {
		return true
	}
	for _, s := range r.ServiceIDs {
		if s == serviceID {
			return true
		}
	}
	return false
}

var langRe = regexp.MustCompile(`^[a-z]{2}(_[A-Z]{2})?$`)

func (in *Rule) check() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		return kit.Invalid("name: 1 to 100 characters")
	}
	switch in.Kind {
	case KindReminder:
		if in.OffsetMinutes < 30 || in.OffsetMinutes > 20160 {
			return kit.Invalid("offset_minutes: 30 minutes to 14 days before")
		}
		in.RecallDays = 0
	case KindRecall:
		if in.RecallDays < 7 || in.RecallDays > 1095 {
			return kit.Invalid("recall_days: 7 to 1,095 days")
		}
		in.OffsetMinutes = 0
	default:
		return kit.Invalid("kind: reminder or recall")
	}
	in.TemplateName = strings.TrimSpace(in.TemplateName)
	if in.TemplateName == "" || len(in.TemplateName) > 255 {
		return kit.Invalid("template_name is required")
	}
	if !langRe.MatchString(in.Language) {
		return kit.Invalid("language: a template language code such as en or en_US")
	}
	if len(in.Variables) > 5 {
		return kit.Invalid("variables: at most 5")
	}
	for _, v := range in.Variables {
		if !variables[v] || (in.Kind == KindRecall && v != "name" && v != "service") {
			return kit.Invalid("variables: %q isn't available for a %s", v, in.Kind)
		}
	}
	if in.ServiceIDs == nil {
		in.ServiceIDs = []string{}
	}
	if in.Variables == nil {
		in.Variables = []string{}
	}
	return nil
}

// checkTemplate makes sure an approved template of that name and language
// exists in the organization (on any WhatsApp number).
func (s *Service) checkTemplate(ctx context.Context, orgID uuid.UUID, name, lang string) error {
	var n int64
	if err := s.db.WithContext(ctx).Model(&models.Template{}).Where("organization_id = ? AND name = ? AND language = ? AND status = ?",
		orgID, name, lang, "APPROVED").Count(&n).Error; err != nil {
		return fmt.Errorf("check template: %w", err)
	}
	if n == 0 {
		return kit.Invalid("template_name: no approved template %q in %s; submit it to Meta first", name, lang)
	}
	return nil
}

// ListRules returns the organization's rules.
func (s *Service) ListRules(ctx context.Context, orgID uuid.UUID) ([]Rule, error) {
	var rows []ruleRow
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Order("kind, offset_minutes DESC, recall_days, name").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	out := make([]Rule, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.view())
	}
	return out, nil
}

// SaveRule creates (id nil) or updates a rule.
func (s *Service) SaveRule(ctx context.Context, orgID, userID uuid.UUID, id *uuid.UUID, in Rule) (*Rule, error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	if err := s.checkTemplate(ctx, orgID, in.TemplateName, in.Language); err != nil {
		return nil, err
	}
	vars, _ := json.Marshal(in.Variables)
	svcs, _ := json.Marshal(in.ServiceIDs)
	now := s.now()
	if id == nil {
		row := ruleRow{OrganizationID: orgID, Kind: in.Kind, Name: in.Name, OffsetMinutes: in.OffsetMinutes, RecallDays: in.RecallDays,
			TemplateName: in.TemplateName, Language: in.Language, Variables: vars, ServiceIDs: svcs, Enabled: in.Enabled,
			CreatedByID: &userID, CreatedAt: now, UpdatedAt: now}
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return nil, fmt.Errorf("create rule: %w", err)
		}
		v := row.view()
		return &v, nil
	}
	res := s.db.WithContext(ctx).Model(&ruleRow{}).Where("organization_id = ? AND id = ?", orgID, *id).Updates(map[string]any{
		"kind": in.Kind, "name": in.Name, "offset_minutes": in.OffsetMinutes, "recall_days": in.RecallDays, "template_name": in.TemplateName,
		"language": in.Language, "variables": vars, "service_ids": svcs, "enabled": in.Enabled, "updated_at": now})
	if res.Error != nil {
		return nil, fmt.Errorf("update rule: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, kit.NotFound("rule not found")
	}
	var row ruleRow
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, *id).First(&row).Error; err != nil {
		return nil, err
	}
	v := row.view()
	return &v, nil
}

// DeleteRule removes a rule and its send history.
func (s *Service) DeleteRule(ctx context.Context, orgID, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, id).Delete(&ruleRow{})
	if res.Error != nil {
		return fmt.Errorf("delete rule: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return kit.NotFound("rule not found")
	}
	return nil
}

// ---------- sends ----------

type sendRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	RuleID         uuid.UUID `gorm:"type:uuid"`
	Provider       string
	TargetID       string
	StartAt        time.Time
	ContactID      *uuid.UUID `gorm:"type:uuid"`
	Status         string
	Reason         string
	MessageID      *uuid.UUID `gorm:"type:uuid"`
	Reply          string
	RepliedAt      *time.Time
	CreatedAt      time.Time
}

func (sendRow) TableName() string { return "wl.reminder_sends" }

// claimSend inserts the send key; false when it was already there (R2).
func (s *Service) claimSend(ctx context.Context, row *sendRow) (bool, error) {
	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	if res.Error != nil {
		return false, fmt.Errorf("claim send: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (s *Service) finish(ctx context.Context, row *sendRow, status, reason string, contactID, messageID *uuid.UUID) {
	if err := s.db.WithContext(ctx).Model(&sendRow{}).Where("id = ?", row.ID).Updates(map[string]any{"status": status, "reason": reason,
		"contact_id": contactID, "message_id": messageID}).Error; err != nil {
		s.log.Error("reminders: send not updated", "organization_id", row.OrganizationID, "error", err)
	}
}

// contactFor finds the Weline contact for a customer by mobile, adding one on
// the organization's default WhatsApp number when needed (R3).
func (s *Service) contactFor(ctx context.Context, orgID uuid.UUID, p provider.Provider, customerID string) (uuid.UUID, error) {
	cust, err := p.GetCustomer(ctx, customerID)
	if err != nil {
		return uuid.Nil, err
	}
	d := strings.TrimPrefix(cust.Mobile, "+")
	if len(d) < 8 {
		return uuid.Nil, errNoMobile
	}
	var cs []models.Contact
	if err := s.db.WithContext(ctx).Select("id").Where("organization_id = ? AND phone_number IN ? AND deleted_at IS NULL", orgID, []string{d, "+" + d}).
		Order("last_message_at DESC NULLS LAST").Limit(1).Find(&cs).Error; err != nil {
		return uuid.Nil, fmt.Errorf("find contact: %w", err)
	}
	if len(cs) == 1 {
		return cs[0].ID, nil
	}
	var accounts []string
	if err := s.db.WithContext(ctx).Model(&models.WhatsAppAccount{}).Where("organization_id = ?", orgID).
		Order("is_default_outgoing DESC, created_at").Limit(1).Pluck("name", &accounts).Error; err != nil {
		return uuid.Nil, fmt.Errorf("load WhatsApp number: %w", err)
	}
	if len(accounts) == 0 {
		return uuid.Nil, errNoAccount
	}
	c := models.Contact{BaseModel: models.BaseModel{ID: uuid.New()}, OrganizationID: orgID, PhoneNumber: d, ProfileName: cust.Name,
		WhatsAppAccount: accounts[0], Metadata: models.JSONB{}}
	if err := s.db.WithContext(ctx).Create(&c).Error; err != nil {
		return uuid.Nil, fmt.Errorf("add contact: %w", err)
	}
	return c.ID, nil
}

var (
	errNoMobile  = errors.New("no_mobile")
	errNoAccount = errors.New("no_whatsapp_number")
)

func firstName(n string) string {
	if f := strings.Fields(n); len(f) > 0 {
		return f[0]
	}
	return "there"
}

func params(vars []string, a *provider.Appointment, name string) map[string]string {
	tz, err := time.LoadLocation(a.TimeZone)
	if err != nil {
		tz = time.UTC
	}
	vals := map[string]string{"name": firstName(name), "service": a.ServiceName, "when": a.Start.In(tz).Format("Mon 2 Jan at 3:04pm"),
		"location": a.LocationName, "practitioner": a.PractitionerName}
	out := map[string]string{}
	for i, v := range vars {
		out[fmt.Sprint(i+1)] = vals[v]
	}
	return out
}

// sendTo resolves the contact, checks consent and sends the template.
func (s *Service) sendTo(ctx context.Context, orgID uuid.UUID, p provider.Provider, row *sendRow, rule Rule, customerID string,
	purpose consent.Purpose, vals func(name string) map[string]string) {
	contactID, err := s.contactFor(ctx, orgID, p, customerID)
	if err != nil {
		reason := "no_contact"
		if errors.Is(err, errNoMobile) || errors.Is(err, errNoAccount) {
			reason = err.Error()
		}
		s.finish(ctx, row, "skipped", reason, nil, nil)
		return
	}
	ok, why, err := s.consent.Allowed(ctx, orgID, contactID, purpose)
	if err != nil || !ok {
		if why == "" {
			why = "consent_unknown"
		}
		s.finish(ctx, row, "skipped", why, &contactID, nil)
		return
	}
	var name string
	var names []string
	_ = s.db.WithContext(ctx).Model(&models.Contact{}).Where("organization_id = ? AND id = ?", orgID, contactID).Pluck("profile_name", &names).Error
	if len(names) == 1 {
		name = names[0]
	}
	msgID, err := s.sender.SendTemplate(ctx, orgID, contactID, rule.TemplateName, rule.Language, vals(name))
	if err != nil {
		s.finish(ctx, row, "failed", "send_failed", &contactID, nil)
		s.log.Error("reminders: template not sent", "organization_id", orgID, "rule_id", rule.ID, "error", err)
		return
	}
	s.finish(ctx, row, "sent", "", &contactID, &msgID)
}

// ---------- the reminder sweep (R2–R5) ----------

// SweepOrg sends the organization's due reminders: appointments from the
// connector whose reminder time has come and that haven't started.
func (s *Service) SweepOrg(ctx context.Context, orgID uuid.UUID) (int, error) {
	rules, err := s.ListRules(ctx, orgID)
	if err != nil {
		return 0, err
	}
	maxOffset := 0
	var active []Rule
	for _, r := range rules {
		if r.Kind == KindReminder && r.Enabled {
			active = append(active, r)
			if r.OffsetMinutes > maxOffset {
				maxOffset = r.OffsetMinutes
			}
		}
	}
	if len(active) == 0 {
		return 0, nil
	}
	p, err := s.providers.For(ctx, orgID)
	if err != nil {
		return 0, err
	}
	now := s.now()
	to := now.Add(time.Duration(maxOffset) * time.Minute)
	appts, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &now, To: &to})
	if err != nil {
		return 0, err
	}
	sent := 0
	for i := range appts {
		a := &appts[i]
		if a.Status != provider.StatusBooked || !a.Start.After(now) {
			continue // cancelled, no-show or already started
		}
		for _, r := range active {
			if !r.appliesTo(a.ServiceID) || now.Before(a.Start.Add(-time.Duration(r.OffsetMinutes)*time.Minute)) {
				continue
			}
			row := &sendRow{OrganizationID: orgID, RuleID: r.ID, Provider: p.Name(), TargetID: a.ID, StartAt: a.Start.UTC(),
				Status: "pending", CreatedAt: now}
			claimed, err := s.claimSend(ctx, row)
			if err != nil {
				return sent, err
			}
			if !claimed {
				continue // already sent for this time
			}
			s.sendTo(ctx, orgID, p, row, r, a.CustomerID, consent.PurposeService, func(name string) map[string]string { return params(r.Variables, a, name) })
			sent++
		}
	}
	return sent, nil
}

// Sweep runs every organization with an enabled reminder rule, on one
// instance at a time (Redis lock).
func (s *Service) Sweep(ctx context.Context) (int, error) {
	if s.rdb != nil {
		ok, err := s.rdb.SetNX(ctx, sweepLock, "1", sweepEvery-30*time.Second).Result()
		if err != nil || !ok {
			return 0, err
		}
	}
	var orgs []uuid.UUID
	if err := s.db.WithContext(ctx).Model(&ruleRow{}).Where("kind = ? AND enabled", KindReminder).Distinct().Pluck("organization_id", &orgs).Error; err != nil {
		return 0, err
	}
	total := 0
	for _, org := range orgs {
		n, err := s.SweepOrg(ctx, org)
		if err != nil {
			s.log.Error("reminders: sweep", "organization_id", org, "error", err)
		}
		total += n
	}
	return total, nil
}

// ---------- replies (R6) ----------

var nonLetters = regexp.MustCompile(`[^a-z]+`)

func replyKind(text string) string {
	switch strings.TrimSpace(nonLetters.ReplaceAllString(strings.ToLower(text), " ")) {
	case "confirm", "confirmed", "yes confirm", "confirm appointment":
		return "confirmed"
	case "cancel", "cancel appointment", "cancel it":
		return "cancelled"
	case "reschedule", "change time", "move", "reschedule appointment":
		return "reschedule"
	}
	return ""
}

// Reply applies a Confirm, Cancel or Reschedule reply to the contact's latest
// unanswered reminder for an appointment that hasn't started. ok is false
// when the message isn't such a reply.
func (s *Service) Reply(ctx context.Context, orgID, contactID uuid.UUID, text string) (reply string, ok bool, err error) {
	kind := replyKind(text)
	if kind == "" {
		return "", false, nil
	}
	now := s.now()
	var rows []sendRow
	if err := s.db.WithContext(ctx).Table("wl.reminder_sends").
		Where("reminder_sends.organization_id = ? AND reminder_sends.contact_id = ? AND reminder_sends.status = 'sent' AND reminder_sends.reply = '' AND reminder_sends.start_at > ?",
			orgID, contactID, now).Joins("JOIN wl.reminder_rules r ON r.id = reminder_sends.rule_id AND r.kind = 'reminder'").
		Order("reminder_sends.created_at DESC").Limit(1).Find(&rows).Error; err != nil {
		return "", false, fmt.Errorf("find reminder: %w", err)
	}
	if len(rows) == 0 {
		return "", false, nil
	}
	row := rows[0]
	p, err := s.providers.For(ctx, orgID)
	if err != nil {
		return "", true, err
	}
	switch kind {
	case "confirmed":
		if c, ok := p.(provider.Confirmer); ok {
			if err := c.Confirm(ctx, row.TargetID); err != nil {
				s.log.Error("reminders: confirm failed", "organization_id", orgID, "error", err)
			}
		}
		reply = "Thanks, you're confirmed. See you then!"
	case "cancelled":
		if err := p.Cancel(ctx, row.TargetID, "Cancelled by the patient from a WhatsApp reminder"); err != nil {
			reply = "Sorry, I couldn't cancel it here. Our team will contact you."
			kind = "reschedule"
			if s.handover != nil {
				_, _ = s.handover.HandOver(orgID, contactID, nil, "Reminder reply: cancellation failed in the diary")
			}
			break
		}
		reply = "Your appointment is cancelled. Reply here anytime to book again."
	case "reschedule":
		if s.handover != nil {
			if _, err := s.handover.HandOver(orgID, contactID, nil, "Reminder reply: wants to reschedule"); err != nil {
				s.log.Error("reminders: hand-over failed", "organization_id", orgID, "error", err)
			}
		}
		reply = "No problem. Our team will message you here to find a new time."
	}
	if err := s.db.WithContext(ctx).Model(&sendRow{}).Where("id = ?", row.ID).Updates(map[string]any{"reply": kind, "replied_at": now}).Error; err != nil {
		return reply, true, fmt.Errorf("save reply: %w", err)
	}
	return reply, true, nil
}

// ---------- recalls (R7, R8) ----------

// Due is a customer due for a recall.
type Due struct {
	CustomerID string    `json:"customer_id"`
	Name       string    `json:"name"`
	Service    string    `json:"service"`
	LastVisit  time.Time `json:"last_visit"`
}

// RecallDue lists customers whose last visit for the rule's services was more
// than recall_days ago (within the year after that), with nothing booked ahead.
func (s *Service) RecallDue(ctx context.Context, orgID, ruleID uuid.UUID) ([]Due, error) {
	r, err := s.rule(ctx, orgID, ruleID, KindRecall)
	if err != nil {
		return nil, err
	}
	p, err := s.providers.For(ctx, orgID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	cutoff := now.AddDate(0, 0, -r.RecallDays)
	from, to := cutoff.AddDate(-1, 0, 0), now.AddDate(1, 0, 0)
	appts, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to})
	if err != nil {
		return nil, err
	}
	last := map[string]*provider.Appointment{}
	ahead := map[string]bool{}
	for i := range appts {
		a := &appts[i]
		switch {
		case a.Status == provider.StatusBooked && a.Start.After(now):
			ahead[a.CustomerID] = true
		case !r.appliesTo(a.ServiceID) || a.Status == provider.StatusCancelled || a.Start.After(now):
		default:
			if l, ok := last[a.CustomerID]; !ok || a.Start.After(l.Start) {
				last[a.CustomerID] = a
			}
		}
	}
	out := []Due{}
	for id, a := range last {
		if ahead[id] || a.Start.After(cutoff) {
			continue
		}
		out = append(out, Due{CustomerID: id, Service: a.ServiceName, LastVisit: a.Start})
	}
	for i := range out {
		if c, err := p.GetCustomer(ctx, out[i].CustomerID); err == nil {
			out[i].Name = c.Name
		}
	}
	return out, nil
}

func (s *Service) rule(ctx context.Context, orgID, id uuid.UUID, kind string) (*Rule, error) {
	var rows []ruleRow
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ? AND kind = ?", orgID, id, kind).Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load rule: %w", err)
	}
	if len(rows) == 0 {
		return nil, kit.NotFound("rule not found")
	}
	v := rows[0].view()
	return &v, nil
}

// SendRecalls sends a recall to the chosen due customers, once per customer,
// rule and month; consent is checked as for marketing (R8, Q2).
func (s *Service) SendRecalls(ctx context.Context, orgID, ruleID uuid.UUID, customerIDs []string) (map[string]int, error) {
	r, err := s.rule(ctx, orgID, ruleID, KindRecall)
	if err != nil {
		return nil, err
	}
	if len(customerIDs) == 0 || len(customerIDs) > 500 {
		return nil, kit.Invalid("customer_ids: 1 to 500")
	}
	due, err := s.RecallDue(ctx, orgID, ruleID)
	if err != nil {
		return nil, err
	}
	byID := map[string]Due{}
	for _, d := range due {
		byID[d.CustomerID] = d
	}
	p, err := s.providers.For(ctx, orgID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	counts := map[string]int{"sent": 0, "skipped": 0, "not_due": 0, "already": 0}
	for _, id := range customerIDs {
		d, ok := byID[id]
		if !ok {
			counts["not_due"]++
			continue
		}
		row := &sendRow{OrganizationID: orgID, RuleID: r.ID, Provider: p.Name(), TargetID: id, StartAt: month, Status: "pending", CreatedAt: now}
		claimed, err := s.claimSend(ctx, row)
		if err != nil {
			return counts, err
		}
		if !claimed {
			counts["already"]++
			continue
		}
		s.sendTo(ctx, orgID, p, row, *r, id, consent.PurposeMarketing, func(name string) map[string]string {
			vals := map[string]string{"name": firstName(name), "service": d.Service}
			out := map[string]string{}
			for i, v := range r.Variables {
				out[fmt.Sprint(i+1)] = vals[v]
			}
			return out
		})
		var sts []string
		_ = s.db.WithContext(ctx).Model(&sendRow{}).Where("id = ?", row.ID).Pluck("status", &sts).Error
		if len(sts) == 1 && sts[0] == "sent" {
			counts["sent"]++
		} else {
			counts["skipped"]++
		}
	}
	return counts, nil
}

// Send is one row of the activity list.
type Send struct {
	ID        uuid.UUID  `json:"id"`
	RuleName  string     `json:"rule_name"`
	Kind      string     `json:"kind"`
	StartAt   time.Time  `json:"start_at"`
	Status    string     `json:"status"`
	Reason    string     `json:"reason"`
	Reply     string     `json:"reply"`
	ContactID *uuid.UUID `json:"contact_id"`
	CreatedAt time.Time  `json:"created_at"`
}

// Sends lists recent sends, newest first.
func (s *Service) Sends(ctx context.Context, orgID uuid.UUID, before *time.Time) ([]Send, error) {
	q := s.db.WithContext(ctx).Table("wl.reminder_sends s").Select("s.id, r.name AS rule_name, r.kind, s.start_at, s.status, s.reason, s.reply, s.contact_id, s.created_at").
		Joins("JOIN wl.reminder_rules r ON r.id = s.rule_id AND r.organization_id = s.organization_id").Where("s.organization_id = ?", orgID)
	if before != nil {
		q = q.Where("s.created_at < ?", *before)
	}
	out := []Send{}
	if err := q.Order("s.created_at DESC, s.id").Limit(50).Scan(&out).Error; err != nil {
		return nil, fmt.Errorf("list sends: %w", err)
	}
	return out, nil
}
