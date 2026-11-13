package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// Outcomes of a run (R8).
const (
	OutcomeAnswered    = "answered"
	OutcomeOffered     = "offered_times"
	OutcomeAsked       = "asked"
	OutcomeBooked      = "booked"
	OutcomeRescheduled = "rescheduled"
	OutcomeCancelled   = "cancelled"
	OutcomeHandedOver  = "handed_over"
	OutcomeEmergency   = "emergency"
	OutcomeIgnored     = "ignored"
	OutcomeError       = "error"

	pendingTTL = 30 * time.Minute
	maxOffer   = 5
	runsKeep   = 90 * 24 * time.Hour
)

// Handover opens an agent transfer (Weline's accessor in handlers).
type Handover interface {
	HasActiveTransfer(orgID, contactID uuid.UUID) bool
	HandOver(orgID, contactID uuid.UUID, teamID *uuid.UUID, notes string) (bool, error)
}

// Logger is the part of Weline's logger the module uses.
type Logger interface {
	Error(msg string, fields ...any)
}

type nopLogger struct{}

func (nopLogger) Error(string, ...any) {}

// Service is the agent for every organization; calls take the organization first.
type Service struct {
	db        *gorm.DB
	rdb       *redis.Client
	llmFor    func(ctx context.Context, orgID uuid.UUID) (kit.LLM, error)
	providers provider.Resolver
	handover  Handover
	sender    kit.Sender
	log       Logger
	now       func() time.Time
	mem       sync.Map // pending choices when there's no Redis (tests)
}

// NewService returns the agent.
func NewService(db *gorm.DB, rdb *redis.Client, llmFor func(context.Context, uuid.UUID) (kit.LLM, error), providers provider.Resolver,
	handover Handover, sender kit.Sender, log Logger) *Service {
	if log == nil {
		log = nopLogger{}
	}
	return &Service{db: db, rdb: rdb, llmFor: llmFor, providers: providers, handover: handover, sender: sender, log: log, now: time.Now}
}

// Result is what the agent decided for one message.
type Result struct {
	Reply      string      `json:"reply"`
	Intent     string      `json:"intent"`
	Confidence float64     `json:"confidence"`
	Outcome    string      `json:"outcome"`
	HandedOver bool        `json:"handed_over"`
	Sources    []uuid.UUID `json:"sources"`
	tokensIn   int
	tokensOut  int
}

// ---------- pending choices ----------

type pendingItem struct {
	Label     string    `json:"label"`
	ID        string    `json:"id"` // service / location / practitioner id
	Start     time.Time `json:"start,omitempty"`
	LocationI string    `json:"location_id,omitempty"`
}

type pending struct {
	Kind          string        `json:"kind"` // service, location, book, reschedule, cancel
	ServiceID     string        `json:"service_id,omitempty"`
	LocationID    string        `json:"location_id,omitempty"`
	AppointmentID string        `json:"appointment_id,omitempty"`
	Items         []pendingItem `json:"items,omitempty"`
}

func pendingKey(orgID, contactID uuid.UUID) string {
	return "wl:ai:pending:" + orgID.String() + ":" + contactID.String()
}

func (s *Service) savePending(ctx context.Context, orgID, contactID uuid.UUID, p *pending) error {
	b, _ := json.Marshal(p)
	if s.rdb == nil {
		s.mem.Store(pendingKey(orgID, contactID), b)
		return nil
	}
	return s.rdb.Set(ctx, pendingKey(orgID, contactID), b, pendingTTL).Err()
}

func (s *Service) takePending(ctx context.Context, orgID, contactID uuid.UUID) *pending {
	var b []byte
	if s.rdb == nil {
		v, ok := s.mem.LoadAndDelete(pendingKey(orgID, contactID))
		if !ok {
			return nil
		}
		b = v.([]byte)
	} else {
		var err error
		if b, err = s.rdb.GetDel(ctx, pendingKey(orgID, contactID)).Bytes(); err != nil {
			return nil
		}
	}
	var p pending
	if json.Unmarshal(b, &p) != nil {
		return nil
	}
	return &p
}

// ---------- the pipeline (R4) ----------

type contactInfo struct {
	Name, Phone string
}

func (s *Service) contact(ctx context.Context, orgID, contactID uuid.UUID) (*contactInfo, error) {
	var cs []models.Contact
	if err := s.db.WithContext(ctx).Select("id", "profile_name", "phone_number").Where("organization_id = ? AND id = ?", orgID, contactID).
		Limit(1).Find(&cs).Error; err != nil {
		return nil, fmt.Errorf("load contact: %w", err)
	}
	if len(cs) == 0 {
		return nil, kit.NotFound("contact not found")
	}
	return &contactInfo{Name: cs[0].ProfileName, Phone: cs[0].PhoneNumber}, nil
}

func (s *Service) businessName(ctx context.Context, orgID uuid.UUID) string {
	var names []string
	_ = s.db.WithContext(ctx).Model(&models.Organization{}).Where("id = ?", orgID).Limit(1).Pluck("name", &names).Error
	if len(names) == 1 && names[0] != "" {
		return names[0]
	}
	return "the clinic"
}

// Decide runs the pipeline for one message from a contact and carries out
// the actions (hand-over, booking), returning the reply to send (R4). It does
// not send.
func (s *Service) Decide(ctx context.Context, orgID, contactID uuid.UUID, text string) (*Result, error) {
	st, err := s.GetSettings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if !st.Enabled || text == "" || (s.handover != nil && s.handover.HasActiveTransfer(orgID, contactID)) {
		return &Result{Outcome: OutcomeIgnored}, nil
	}
	lower := strings.ToLower(text)

	// 1. Emergency words, before any model call.
	for _, w := range st.EmergencyWords {
		if containsWord(lower, w) {
			r := &Result{Reply: emergencyReply, Intent: IntentClinical, Confidence: 1, Outcome: OutcomeEmergency}
			return r, s.handOver(ctx, orgID, contactID, st, r, "AI: possible emergency")
		}
	}
	if lower == "human" || lower == "person" || lower == "agent" {
		r := &Result{Reply: handoverReply, Intent: IntentOther, Confidence: 1}
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: patient asked for a person")
	}

	// 2. A pending choice.
	if p := s.takePending(ctx, orgID, contactID); p != nil {
		if r, ok, err := s.complete(ctx, orgID, contactID, st, p, lower); ok || err != nil {
			return r, err
		}
	}

	// 3. Classify.
	llm, err := s.llmFor(ctx, orgID)
	if err != nil {
		r := &Result{Reply: handoverReply, Intent: IntentOther}
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: not available ("+safeErr(err)+")")
	}
	business := s.businessName(ctx, orgID)
	resp, err := llm.Complete(ctx, kit.LLMRequest{System: classifyPrompt(business, st.Instructions),
		Messages: []kit.LLMMessage{{Role: "user", Content: text}}, MaxTokens: 100})
	if err != nil {
		r := &Result{Reply: handoverReply, Intent: IntentOther, Outcome: OutcomeError}
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: model unavailable")
	}
	var c struct {
		Intent     string  `json:"intent"`
		Confidence float64 `json:"confidence"`
		Service    string  `json:"service"`
	}
	r := &Result{tokensIn: resp.TokensIn, tokensOut: resp.TokensOut}
	if err := parseJSON(resp.Text, &c); err != nil || !intents[c.Intent] {
		r.Reply, r.Intent = handoverReply, IntentOther
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: couldn't understand the message")
	}
	r.Intent, r.Confidence = c.Intent, c.Confidence

	// 4. Hand over what the agent mustn't handle.
	switch {
	case c.Intent == IntentClinical:
		r.Reply = clinicalReply
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: clinical question")
	case c.Intent == IntentComplaint || c.Intent == IntentOther:
		r.Reply = handoverReply
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: "+c.Intent)
	case c.Confidence < st.ConfidenceThreshold:
		r.Reply = handoverReply
		return r, s.handOver(ctx, orgID, contactID, st, r, fmt.Sprintf("AI: unsure (%s, %.2f)", c.Intent, c.Confidence))
	}

	// 5–7.
	switch c.Intent {
	case IntentQuestion:
		return s.answer(ctx, orgID, contactID, st, llm, business, text, r)
	case IntentBook:
		return s.startBooking(ctx, orgID, contactID, st, c.Service, r)
	default:
		return s.startChange(ctx, orgID, contactID, st, c.Intent, r)
	}
}

func containsWord(text, phrase string) bool {
	i := strings.Index(text, phrase)
	for i >= 0 {
		before := i == 0 || !isWordChar(text[i-1])
		after := i+len(phrase) == len(text) || !isWordChar(text[i+len(phrase)])
		if before && after {
			return true
		}
		next := strings.Index(text[i+1:], phrase)
		if next < 0 {
			return false
		}
		i += next + 1
	}
	return false
}

func isWordChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}

func safeErr(err error) string {
	var e *kit.Error
	if errors.As(err, &e) {
		return e.Message
	}
	return "error"
}

// parseJSON reads the model's JSON, tolerating a code fence around it.
func parseJSON(s string, v any) error {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "```json"), "```")
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	return json.Unmarshal([]byte(s), v)
}

func (s *Service) handOver(ctx context.Context, orgID, contactID uuid.UUID, st Settings, r *Result, notes string) error {
	if r.Outcome == "" {
		r.Outcome = OutcomeHandedOver
	}
	if s.handover == nil {
		return nil
	}
	if _, err := s.handover.HandOver(orgID, contactID, st.HandoverTeamID, notes); err != nil {
		// The reply still tells the patient, and staff see the chat.
		s.log.Error("ai: hand-over failed", "organization_id", orgID, "contact_id", contactID, "error", err)
		return nil
	}
	r.HandedOver = true // created now, or a person already had it
	return nil
}

// answer replies to a question from the knowledge base only (R4.5).
func (s *Service) answer(ctx context.Context, orgID, contactID uuid.UUID, st Settings, llm kit.LLM, business, text string, r *Result) (*Result, error) {
	passages, err := s.Search(ctx, orgID, text)
	if err != nil {
		return nil, err
	}
	if len(passages) == 0 {
		r.Reply = handoverReply
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: question not in the knowledge base")
	}
	resp, err := llm.Complete(ctx, kit.LLMRequest{System: answerPrompt(business, st.Tone, st.Instructions, passages),
		Messages: []kit.LLMMessage{{Role: "user", Content: text}}, MaxTokens: 300})
	if err != nil {
		r.Reply, r.Outcome = handoverReply, OutcomeError
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: model unavailable")
	}
	r.tokensIn += resp.TokensIn
	r.tokensOut += resp.TokensOut
	var a struct {
		CanAnswer bool   `json:"can_answer"`
		Answer    string `json:"answer"`
		Sources   []int  `json:"sources"`
	}
	if err := parseJSON(resp.Text, &a); err != nil || !a.CanAnswer || strings.TrimSpace(a.Answer) == "" || hasAdvice(a.Answer) {
		r.Reply = handoverReply
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: couldn't answer from the knowledge base")
	}
	for _, n := range a.Sources {
		if n >= 1 && n <= len(passages) {
			r.Sources = append(r.Sources, passages[n-1].SourceID)
		}
	}
	if len(r.Sources) == 0 {
		r.Sources = []uuid.UUID{passages[0].SourceID}
	}
	r.Reply, r.Outcome = strings.TrimSpace(a.Answer), OutcomeAnswered
	return r, nil
}

// ---------- booking conversation (R4.6, R4.7) ----------

func (s *Service) provider(ctx context.Context, orgID uuid.UUID) (provider.Provider, error) {
	if s.providers == nil {
		return nil, provider.ErrNotConnected
	}
	return s.providers.For(ctx, orgID)
}

func numbered(intro string, items []pendingItem) string {
	var b strings.Builder
	b.WriteString(intro)
	for i, it := range items {
		fmt.Fprintf(&b, "\n%d) %s", i+1, it.Label)
	}
	b.WriteString(askNumberSuffix)
	return b.String()
}

func (s *Service) startBooking(ctx context.Context, orgID, contactID uuid.UUID, st Settings, serviceHint string, r *Result) (*Result, error) {
	p, err := s.provider(ctx, orgID)
	if err != nil {
		r.Reply = handoverReply
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: booking unavailable")
	}
	svcs, err := p.ListServices(ctx)
	if err != nil || len(svcs) == 0 {
		r.Reply = handoverReply
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: no services to book")
	}
	var chosen *provider.Service
	hint := strings.ToLower(strings.TrimSpace(serviceHint))
	for i := range svcs {
		n := strings.ToLower(svcs[i].Name)
		if len(svcs) == 1 || (hint != "" && (strings.Contains(n, hint) || strings.Contains(hint, n))) {
			chosen = &svcs[i]
			break
		}
	}
	if chosen == nil {
		items := []pendingItem{}
		for i, sv := range svcs {
			if i == maxOffer*2 {
				break
			}
			items = append(items, pendingItem{Label: sv.Name, ID: sv.ID})
		}
		r.Reply, r.Outcome = numbered("Which appointment would you like?", items), OutcomeAsked
		return r, s.savePending(ctx, orgID, contactID, &pending{Kind: "service", Items: items})
	}
	return s.chooseLocation(ctx, orgID, contactID, st, p, chosen.ID, r)
}

func (s *Service) chooseLocation(ctx context.Context, orgID, contactID uuid.UUID, st Settings, p provider.Provider, serviceID string, r *Result) (*Result, error) {
	locs, err := p.ListLocations(ctx)
	if err != nil || len(locs) == 0 {
		r.Reply = handoverReply
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: no locations to book")
	}
	if len(locs) > 1 {
		items := []pendingItem{}
		for _, l := range locs {
			items = append(items, pendingItem{Label: l.Name, ID: l.ID})
		}
		r.Reply, r.Outcome = numbered("Which location suits you?", items), OutcomeAsked
		return r, s.savePending(ctx, orgID, contactID, &pending{Kind: "location", ServiceID: serviceID, Items: items})
	}
	return s.offerTimes(ctx, orgID, contactID, st, p, serviceID, locs[0], "", r)
}

func (s *Service) offerTimes(ctx context.Context, orgID, contactID uuid.UUID, st Settings, p provider.Provider, serviceID string,
	loc provider.Location, appointmentID string, r *Result) (*Result, error) {
	free, err := p.FindFreeTimes(ctx, provider.FreeTimeQuery{ServiceID: serviceID, LocationID: loc.ID, Patient: true, Limit: maxOffer})
	if err != nil || len(free.Times) == 0 {
		r.Reply = noTimesReply
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: no free times found")
	}
	tz, err := time.LoadLocation(loc.TimeZone)
	if err != nil {
		tz = time.UTC
	}
	items := make([]pendingItem, 0, len(free.Times))
	for _, ft := range free.Times {
		items = append(items, pendingItem{Label: ft.Start.In(tz).Format("Mon 2 Jan 3:04pm") + " with " + ft.PractitionerName,
			ID: ft.PractitionerID, Start: ft.Start, LocationI: loc.ID})
	}
	kind, intro := "book", "Here are the next free times:"
	if appointmentID != "" {
		kind, intro = "reschedule", "Here are times you can move to:"
	}
	r.Reply, r.Outcome = numbered(intro, items), OutcomeOffered
	return r, s.savePending(ctx, orgID, contactID, &pending{Kind: kind, ServiceID: serviceID, LocationID: loc.ID, AppointmentID: appointmentID, Items: items})
}

// startChange finds the contact's next appointment to move or cancel.
func (s *Service) startChange(ctx context.Context, orgID, contactID uuid.UUID, st Settings, intent string, r *Result) (*Result, error) {
	p, err := s.provider(ctx, orgID)
	if err != nil {
		r.Reply = handoverReply
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: booking unavailable")
	}
	appt, err := s.nextAppointment(ctx, orgID, contactID, p)
	if err != nil || appt == nil {
		r.Reply = noApptReply
		return r, s.handOver(ctx, orgID, contactID, st, r, "AI: no upcoming appointment found for "+intent)
	}
	tz, _ := time.LoadLocation(appt.TimeZone)
	if tz == nil {
		tz = time.UTC
	}
	when := appt.Start.In(tz).Format("Mon 2 Jan 3:04pm")
	if intent == IntentCancel {
		r.Reply, r.Outcome = fmt.Sprintf("Do you want to cancel your %s on %s? Reply YES to cancel or NO to keep it.", appt.ServiceName, when), OutcomeAsked
		return r, s.savePending(ctx, orgID, contactID, &pending{Kind: "cancel", AppointmentID: appt.ID,
			Items: []pendingItem{{Label: appt.ServiceName, ID: when}}})
	}
	locs, err := p.ListLocations(ctx)
	if err != nil {
		return nil, err
	}
	loc := provider.Location{ID: appt.LocationID, Name: appt.LocationName, TimeZone: appt.TimeZone}
	for _, l := range locs {
		if l.ID == appt.LocationID {
			loc = l
		}
	}
	return s.offerTimes(ctx, orgID, contactID, st, p, appt.ServiceID, loc, appt.ID, r)
}

func (s *Service) customer(ctx context.Context, orgID, contactID uuid.UUID, p provider.Provider) (*provider.Customer, error) {
	c, err := s.contact(ctx, orgID, contactID)
	if err != nil {
		return nil, err
	}
	cust, _, err := p.FindOrCreateCustomer(ctx, provider.CustomerInput{ContactID: contactID, Name: c.Name, Mobile: c.Phone})
	return cust, err
}

func (s *Service) nextAppointment(ctx context.Context, orgID, contactID uuid.UUID, p provider.Provider) (*provider.Appointment, error) {
	cust, err := s.customer(ctx, orgID, contactID, p)
	if err != nil {
		return nil, err
	}
	from, to := s.now(), s.now().AddDate(0, 4, 0)
	appts, err := p.ListAppointments(ctx, provider.AppointmentQuery{From: &from, To: &to, CustomerID: cust.ID})
	if err != nil {
		return nil, err
	}
	var next *provider.Appointment
	for i := range appts {
		if appts[i].Status == provider.StatusBooked && (next == nil || appts[i].Start.Before(next.Start)) {
			next = &appts[i]
		}
	}
	return next, nil
}

// complete finishes a pending choice. ok is false when the message isn't a
// valid choice, so it's handled as a new message.
func (s *Service) complete(ctx context.Context, orgID, contactID uuid.UUID, st Settings, pd *pending, text string) (*Result, bool, error) {
	r := &Result{Intent: map[string]string{"service": IntentBook, "location": IntentBook, "book": IntentBook,
		"reschedule": IntentReschedule, "cancel": IntentCancel}[pd.Kind], Confidence: 1}
	p, err := s.provider(ctx, orgID)
	if err != nil {
		return nil, false, err
	}
	if pd.Kind == "cancel" {
		switch strings.Trim(text, " .!") {
		case "yes", "y", "yes please":
			if err := p.Cancel(ctx, pd.AppointmentID, "Cancelled by the patient through the AI assistant"); err != nil {
				r.Reply = handoverReply
				return r, true, s.handOver(ctx, orgID, contactID, st, r, "AI: cancellation failed")
			}
			r.Reply, r.Outcome = fmt.Sprintf(cancelledReply, pd.Items[0].Label, pd.Items[0].ID), OutcomeCancelled
			return r, true, nil
		case "no", "n", "keep":
			r.Reply, r.Outcome = keptReply, OutcomeAnswered
			return r, true, nil
		}
		return nil, false, nil
	}
	n, err := strconv.Atoi(strings.Trim(text, " .)"))
	if err != nil || n < 1 || n > len(pd.Items) {
		return nil, false, nil
	}
	it := pd.Items[n-1]
	switch pd.Kind {
	case "service":
		res, err := s.chooseLocation(ctx, orgID, contactID, st, p, it.ID, r)
		return res, true, err
	case "location":
		locs, err := p.ListLocations(ctx)
		if err != nil {
			return nil, true, err
		}
		for _, l := range locs {
			if l.ID == it.ID {
				res, err := s.offerTimes(ctx, orgID, contactID, st, p, pd.ServiceID, l, "", r)
				return res, true, err
			}
		}
		return nil, false, nil
	case "book":
		cust, err := s.customer(ctx, orgID, contactID, p)
		if err != nil {
			r.Reply = handoverReply
			return r, true, s.handOver(ctx, orgID, contactID, st, r, "AI: couldn't find the patient in the diary")
		}
		a, err := p.Book(ctx, provider.BookInput{ServiceID: pd.ServiceID, LocationID: pd.LocationID, PractitionerID: it.ID, Start: it.Start,
			CustomerID: cust.ID, ContactID: contactID, Source: provider.SourceAgent})
		if errors.Is(err, provider.ErrSlotTaken) {
			r.Reply, r.Outcome = "Sorry, that time was just taken. Say \"book\" to see the latest times.", OutcomeAnswered
			return r, true, nil
		}
		if err != nil {
			r.Reply = handoverReply
			return r, true, s.handOver(ctx, orgID, contactID, st, r, "AI: booking failed")
		}
		tz, _ := time.LoadLocation(a.TimeZone)
		if tz == nil {
			tz = time.UTC
		}
		r.Reply, r.Outcome = fmt.Sprintf(bookedReply, a.ServiceName, a.PractitionerName, a.Start.In(tz).Format("Mon 2 Jan 3:04pm"), a.LocationName, a.Reference), OutcomeBooked
		return r, true, nil
	case "reschedule":
		a, err := p.Reschedule(ctx, pd.AppointmentID, provider.RescheduleInput{Start: it.Start, PractitionerID: it.ID})
		if errors.Is(err, provider.ErrSlotTaken) {
			r.Reply, r.Outcome = "Sorry, that time was just taken. Say \"move my appointment\" to see the latest times.", OutcomeAnswered
			return r, true, nil
		}
		if err != nil {
			r.Reply = handoverReply
			return r, true, s.handOver(ctx, orgID, contactID, st, r, "AI: rescheduling failed")
		}
		tz, _ := time.LoadLocation(a.TimeZone)
		if tz == nil {
			tz = time.UTC
		}
		r.Reply, r.Outcome = fmt.Sprintf(rescheduledReply, a.ServiceName, a.Start.In(tz).Format("Mon 2 Jan 3:04pm"), a.PractitionerName), OutcomeRescheduled
		return r, true, nil
	}
	return nil, false, nil
}

// ---------- running on a message ----------

type runRow struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID  `gorm:"type:uuid"`
	ContactID      uuid.UUID  `gorm:"type:uuid"`
	MessageID      *uuid.UUID `gorm:"type:uuid"`
	Intent         string
	Confidence     float64
	Outcome        string
	Sources        json.RawMessage `gorm:"type:jsonb"`
	PromptVersion  string
	LatencyMs      int
	TokensIn       int
	TokensOut      int
	CreatedAt      time.Time
}

func (runRow) TableName() string { return "wl.ai_runs" }

// label marks a reply as automated (R6): the robot on every reply, and a note
// on the first reply to this contact in 24 hours.
func (s *Service) label(ctx context.Context, orgID, contactID uuid.UUID, reply string) string {
	if reply == "" {
		return ""
	}
	var n int64
	_ = s.db.WithContext(ctx).Model(&runRow{}).Where("organization_id = ? AND contact_id = ? AND created_at > ? AND outcome <> ?",
		orgID, contactID, s.now().Add(-24*time.Hour), OutcomeIgnored).Count(&n).Error
	out := labelPrefix + reply
	if n == 0 {
		out += firstReplyNote
	}
	return out
}

// Run decides on a message, records the run and returns the labelled reply.
// A message already handled (same id) is ignored (idempotent).
func (s *Service) Run(ctx context.Context, orgID, contactID uuid.UUID, messageID *uuid.UUID, text string) (*Result, error) {
	if messageID != nil {
		var n int64
		if err := s.db.WithContext(ctx).Model(&runRow{}).Where("organization_id = ? AND message_id = ?", orgID, *messageID).Count(&n).Error; err != nil {
			return nil, err
		}
		if n > 0 {
			return &Result{Outcome: OutcomeIgnored}, nil
		}
	}
	start := s.now()
	r, err := s.Decide(ctx, orgID, contactID, text)
	if err != nil {
		return nil, err
	}
	if r.Outcome == OutcomeIgnored {
		return r, nil
	}
	r.Reply = s.label(ctx, orgID, contactID, r.Reply)
	src, _ := json.Marshal(append([]uuid.UUID{}, r.Sources...))
	row := runRow{OrganizationID: orgID, ContactID: contactID, MessageID: messageID, Intent: r.Intent, Confidence: r.Confidence,
		Outcome: r.Outcome, Sources: src, PromptVersion: PromptVersion, LatencyMs: int(s.now().Sub(start).Milliseconds()),
		TokensIn: r.tokensIn, TokensOut: r.tokensOut, CreatedAt: s.now()}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		s.log.Error("ai: run not recorded", "organization_id", orgID, "error", err)
	}
	return r, nil
}

// PurgeRuns deletes runs older than 90 days (R8).
func (s *Service) PurgeRuns(ctx context.Context) (int64, error) {
	res := s.db.WithContext(ctx).Where("created_at < ?", s.now().Add(-runsKeep)).Delete(&runRow{})
	return res.RowsAffected, res.Error
}
