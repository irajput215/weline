package booking

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Service holds the booking rules (requirements R1–R23). Every method takes
// the organization first and filters every query by it.
type Service struct {
	db       *gorm.DB
	practice *practice.Service
	offers   offerStore
	now      func() time.Time
}

// NewService returns the booking service. It reads the clinic through the
// practice module's service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db, practice: practice.NewService(db), offers: &memoryOffers{}, now: time.Now}
}

// WithRedis keeps chatbot offers in Redis, so they survive restarts and are
// shared between server instances.
func (s *Service) WithRedis(rdb *redis.Client) *Service {
	if rdb != nil {
		s.offers = redisOffers{rdb: rdb}
	}
	return s
}

// Limits.
const (
	maxRangeDays  = 31
	maxSlots      = 200
	noteMaxLen    = 280
	reasonMaxLen  = 200
	blockReasonLn = 120
	labelLayout   = "Mon 2 Jan 15:04"
	localLayout   = "2006-01-02T15:04"
)

// errNotSetUp is returned while the clinic profile is missing.
var errNotSetUp = kit.Conflict("practice not set up: save the clinic profile first")

func isSQLState(err error, code string) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE "+code)
}

// clinic is what most operations need about the organization's practice.
type clinic struct {
	profile  *practice.Profile
	tz       *time.Location
	settings Settings
}

func (s *Service) clinic(ctx context.Context, orgID uuid.UUID) (*clinic, error) {
	prof, err := s.practice.GetProfile(ctx, orgID)
	if practice.IsNotSetUp(err) {
		return nil, errNotSetUp
	}
	if err != nil {
		return nil, err
	}
	tz, err := prof.Location()
	if err != nil {
		return nil, fmt.Errorf("clinic time zone %q: %w", prof.TimeZone, err)
	}
	set, err := s.GetSettings(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return &clinic{profile: prof, tz: tz, settings: *set}, nil
}

func (c *clinic) local(t time.Time) time.Time { return t.In(c.tz) }

// day returns local midnight of a date in the clinic's zone.
func (c *clinic) day(d practice.Date) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, c.tz)
}

func (c *clinic) today(now time.Time) practice.Date {
	l := now.In(c.tz)
	return practice.Date{Time: time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)}
}

// ---------- settings ----------

// GetSettings returns the clinic's booking rules, or the defaults.
func (s *Service) GetSettings(ctx context.Context, orgID uuid.UUID) (*Settings, error) {
	var found []Settings
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Limit(1).Find(&found).Error; err != nil {
		return nil, fmt.Errorf("load booking settings: %w", err)
	}
	if len(found) == 0 {
		return &Settings{OrganizationID: orgID, MinNoticeMinutes: DefaultMinNoticeMinutes, WindowDays: DefaultWindowDays}, nil
	}
	return &found[0], nil
}

// SaveSettings stores the clinic's booking rules (Q4).
func (s *Service) SaveSettings(ctx context.Context, orgID uuid.UUID, minNotice, windowDays int) (*Settings, error) {
	if minNotice < 0 || minNotice > 10080 {
		return nil, kit.Invalid("minimum notice must be between 0 and 10080 minutes (7 days)")
	}
	if windowDays < 1 || windowDays > 365 {
		return nil, kit.Invalid("booking window must be between 1 and 365 days")
	}
	set := Settings{OrganizationID: orgID, MinNoticeMinutes: minNotice, WindowDays: windowDays, UpdatedAt: s.now()}
	err := s.db.WithContext(ctx).Exec(`INSERT INTO wl.booking_settings (organization_id, min_notice_minutes, window_days)
		VALUES (?, ?, ?) ON CONFLICT (organization_id) DO UPDATE
		SET min_notice_minutes = EXCLUDED.min_notice_minutes, window_days = EXCLUDED.window_days, updated_at = now()`,
		orgID, minNotice, windowDays).Error
	if err != nil {
		return nil, fmt.Errorf("save booking settings: %w", err)
	}
	return &set, nil
}

// ---------- availability (R1, R3) ----------

// GetAvailability returns a practitioner's weekly working blocks.
func (s *Service) GetAvailability(ctx context.Context, orgID, practitionerID uuid.UUID) ([]Availability, error) {
	if _, err := s.practice.GetPractitioner(ctx, orgID, practitionerID); err != nil {
		return nil, err
	}
	var rows []availabilityRow
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND practitioner_id = ?", orgID, practitionerID).
		Order("weekday, starts_minute").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load availability: %w", err)
	}
	out := make([]Availability, 0, len(rows))
	for _, r := range rows {
		out = append(out, Availability{LocationID: r.LocationID, Weekday: r.Weekday,
			StartsAt: formatMinute(r.StartsMinute), EndsAt: formatMinute(r.EndsMinute)})
	}
	return out, nil
}

// SetAvailability replaces a practitioner's week. Blocks must be on the slot
// grid, end after they start, not overlap, and be at a location the
// practitioner works at. Parts outside opening hours are kept but come back as
// warnings: they are never offered (R3).
func (s *Service) SetAvailability(ctx context.Context, orgID, practitionerID uuid.UUID, in []Availability) ([]Availability, []string, error) {
	c, err := s.clinic(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	pr, err := s.practice.GetPractitioner(ctx, orgID, practitionerID)
	if err != nil {
		return nil, nil, err
	}
	worksAt := map[uuid.UUID]bool{}
	for _, id := range pr.LocationIDs {
		worksAt[id] = true
	}
	grid := c.profile.SlotMinutes
	rows := make([]availabilityRow, 0, len(in))
	for _, a := range in {
		if a.Weekday < 1 || a.Weekday > 7 {
			return nil, nil, kit.Invalid("weekday must be 1 (Monday) to 7 (Sunday)")
		}
		if !worksAt[a.LocationID] {
			return nil, nil, kit.Invalid("%s doesn't work at that location; add it to the practitioner first", pr.DisplayName)
		}
		from, err1 := parseMinute(a.StartsAt)
		to, err2 := parseMinute(a.EndsAt)
		if err1 != nil || err2 != nil {
			return nil, nil, kit.Invalid("times must be HH:MM")
		}
		if to <= from {
			return nil, nil, kit.Invalid("%s %s–%s: the end must be after the start", weekdayName(a.Weekday), a.StartsAt, a.EndsAt)
		}
		if from%grid != 0 || to%grid != 0 {
			return nil, nil, kit.Invalid("%s %s–%s is not on the clinic's %d-minute grid", weekdayName(a.Weekday), a.StartsAt, a.EndsAt, grid)
		}
		rows = append(rows, availabilityRow{OrganizationID: orgID, PractitionerID: practitionerID, LocationID: a.LocationID,
			Weekday: a.Weekday, StartsMinute: from, EndsMinute: to})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Weekday != rows[j].Weekday {
			return rows[i].Weekday < rows[j].Weekday
		}
		return rows[i].StartsMinute < rows[j].StartsMinute
	})
	for i := 1; i < len(rows); i++ {
		if rows[i].Weekday == rows[i-1].Weekday && rows[i].StartsMinute < rows[i-1].EndsMinute {
			return nil, nil, kit.Invalid("%s %s–%s overlaps %s–%s", weekdayName(rows[i].Weekday),
				formatMinute(rows[i].StartsMinute), formatMinute(rows[i].EndsMinute),
				formatMinute(rows[i-1].StartsMinute), formatMinute(rows[i-1].EndsMinute))
		}
	}

	warnings, err := s.outsideHoursWarnings(ctx, orgID, rows)
	if err != nil {
		return nil, nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ? AND practitioner_id = ?", orgID, practitionerID).Delete(&availabilityRow{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		return nil, nil, fmt.Errorf("save availability: %w", err)
	}
	out, err := s.GetAvailability(ctx, orgID, practitionerID)
	return out, warnings, err
}

func (s *Service) outsideHoursWarnings(ctx context.Context, orgID uuid.UUID, rows []availabilityRow) ([]string, error) {
	warnings := []string{}
	hours := map[uuid.UUID]map[int]practice.MinuteRange{}
	names := map[uuid.UUID]string{}
	for _, r := range rows {
		if _, ok := hours[r.LocationID]; !ok {
			h, err := s.practice.OpeningMinutes(ctx, orgID, r.LocationID)
			if err != nil {
				return nil, err
			}
			hours[r.LocationID] = h
			loc, err := s.practice.GetLocation(ctx, orgID, r.LocationID)
			if err != nil {
				return nil, err
			}
			names[r.LocationID] = loc.Name
		}
		open, ok := hours[r.LocationID][r.Weekday]
		if !ok || r.StartsMinute < open.Start || r.EndsMinute > open.End {
			warnings = append(warnings, fmt.Sprintf("%s %s–%s at %s is outside opening hours; only the open part is offered",
				weekdayName(r.Weekday), formatMinute(r.StartsMinute), formatMinute(r.EndsMinute), names[r.LocationID]))
		}
	}
	return warnings, nil
}

// ---------- blocks (R2) ----------

// ListBlocks returns blocks overlapping [from, to), optionally for one practitioner.
func (s *Service) ListBlocks(ctx context.Context, orgID uuid.UUID, practitionerID *uuid.UUID, from, to time.Time) ([]Block, error) {
	q := s.db.WithContext(ctx).Where("organization_id = ? AND starts_at < ? AND ends_at > ?", orgID, to, from)
	if practitionerID != nil {
		q = q.Where("practitioner_id = ?", *practitionerID)
	}
	out := []Block{}
	if err := q.Order("starts_at").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("load blocks: %w", err)
	}
	return out, nil
}

// CreateBlock blocks a practitioner's time. Appointments already inside the
// block are returned so staff can move them; they are not cancelled.
func (s *Service) CreateBlock(ctx context.Context, orgID uuid.UUID, actor Actor, practitionerID uuid.UUID, from, to time.Time, reason string) (*Block, []Appointment, error) {
	if _, err := s.practice.GetPractitioner(ctx, orgID, practitionerID); err != nil {
		return nil, nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, nil, kit.Invalid("reason is required")
	}
	if len([]rune(reason)) > blockReasonLn {
		return nil, nil, kit.Invalid("reason is too long (max %d characters)", blockReasonLn)
	}
	if !to.After(from) {
		return nil, nil, kit.Invalid("the end must be after the start")
	}
	b := Block{OrganizationID: orgID, PractitionerID: practitionerID, StartsAt: from.UTC(), EndsAt: to.UTC(), Reason: reason, CreatedByID: actor.UserID}
	if err := s.db.WithContext(ctx).Create(&b).Error; err != nil {
		return nil, nil, fmt.Errorf("save block: %w", err)
	}
	clashes, err := s.List(ctx, orgID, ListQuery{rangeFrom: &b.StartsAt, rangeTo: &b.EndsAt, PractitionerID: &practitionerID, activeOnly: true})
	if err != nil {
		return nil, nil, err
	}
	return &b, clashes, nil
}

// DeleteBlock removes a block, freeing the time again.
func (s *Service) DeleteBlock(ctx context.Context, orgID, id uuid.UUID) error {
	res := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, id).Delete(&Block{})
	if res.Error != nil {
		return fmt.Errorf("delete block: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return kit.NotFound("block not found")
	}
	return nil
}

// ---------- slot search (R4–R6) ----------

// SlotQuery asks for free times.
type SlotQuery struct {
	ServiceID      uuid.UUID
	LocationID     uuid.UUID
	PractitionerID *uuid.UUID // nil = anyone
	From, To       practice.Date
	// Patient applies the clinic's minimum notice and booking window (web,
	// WhatsApp). Staff only can't book in the past.
	Patient bool
	Limit   int
	// ignore is an appointment whose own time doesn't count as busy (moving it).
	ignore *uuid.UUID
}

// SearchSlots returns free start times for a service at a location.
func (s *Service) SearchSlots(ctx context.Context, orgID uuid.UUID, q SlotQuery) ([]Slot, error) {
	c, err := s.clinic(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return s.searchSlots(ctx, orgID, c, q)
}

type bookable struct {
	service   *practice.ClinicService
	location  *practice.Location
	candidate []practice.Practitioner
}

// bookable checks the service, location and practitioner can go together, and
// returns the practitioners who may provide the service there.
func (s *Service) bookable(ctx context.Context, orgID, serviceID, locationID uuid.UUID, practitionerID *uuid.UUID) (*bookable, error) {
	svc, err := s.practice.GetService(ctx, orgID, serviceID)
	if err != nil {
		return nil, err
	}
	if !svc.IsActive {
		return nil, kit.Invalid("%s is not offered at the moment", svc.Name)
	}
	loc, err := s.practice.GetLocation(ctx, orgID, locationID)
	if err != nil {
		return nil, err
	}
	if !loc.IsActive {
		return nil, kit.Invalid("%s is not taking bookings", loc.Name)
	}
	allowed := map[string]bool{}
	for _, r := range svc.AllowedRoles {
		allowed[strings.ToLower(r)] = true
	}
	fits := func(p practice.Practitioner) error {
		if !p.IsActive {
			return kit.Invalid("%s is not taking bookings", p.DisplayName)
		}
		if !allowed[strings.ToLower(p.Role)] {
			return kit.Invalid("%s (%s) doesn't provide %s", p.DisplayName, p.Role, svc.Name)
		}
		for _, l := range p.LocationIDs {
			if l == locationID {
				return nil
			}
		}
		return kit.Invalid("%s doesn't work at %s", p.DisplayName, loc.Name)
	}
	b := &bookable{service: svc, location: loc}
	if practitionerID != nil {
		p, err := s.practice.GetPractitioner(ctx, orgID, *practitionerID)
		if err != nil {
			return nil, err
		}
		if err := fits(*p); err != nil {
			return nil, err
		}
		b.candidate = []practice.Practitioner{*p}
		return b, nil
	}
	all, err := s.practice.ActivePractitioners(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, p := range all {
		if fits(p) == nil {
			b.candidate = append(b.candidate, p)
		}
	}
	return b, nil
}

func (s *Service) searchSlots(ctx context.Context, orgID uuid.UUID, c *clinic, q SlotQuery) ([]Slot, error) {
	if q.To.IsZero() {
		q.To = q.From
	}
	if q.To.Before(q.From.Time) {
		return nil, kit.Invalid("'to' must not be before 'from'")
	}
	if q.To.Sub(q.From.Time) > (maxRangeDays-1)*24*time.Hour {
		return nil, kit.Invalid("search at most %d days at a time", maxRangeDays)
	}
	b, err := s.bookable(ctx, orgID, q.ServiceID, q.LocationID, q.PractitionerID)
	if err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 || limit > maxSlots {
		limit = maxSlots
	}
	if len(b.candidate) == 0 {
		return []Slot{}, nil
	}

	now := s.now()
	earliest, latest := now, c.day(q.To).AddDate(0, 0, 1)
	if q.Patient {
		earliest = now.Add(time.Duration(c.settings.MinNoticeMinutes) * time.Minute)
		if w := now.AddDate(0, 0, c.settings.WindowDays); w.Before(latest) {
			latest = w
		}
	}
	rangeFrom, rangeTo := c.day(q.From), c.day(q.To).AddDate(0, 0, 1)

	ids := make([]uuid.UUID, 0, len(b.candidate))
	for _, p := range b.candidate {
		ids = append(ids, p.ID)
	}
	opening, err := s.practice.OpeningMinutes(ctx, orgID, q.LocationID)
	if err != nil {
		return nil, err
	}
	closed, err := s.practice.ClosedDays(ctx, orgID, q.LocationID, q.From, q.To)
	if err != nil {
		return nil, err
	}
	var avail []availabilityRow
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND location_id = ? AND practitioner_id IN ?", orgID, q.LocationID, ids).
		Find(&avail).Error; err != nil {
		return nil, fmt.Errorf("load availability: %w", err)
	}
	busy, err := s.busy(ctx, orgID, ids, rangeFrom, rangeTo, q.ignore)
	if err != nil {
		return nil, err
	}

	byStart := map[int64]*Slot{}
	for d := q.From.Time; !d.After(q.To.Time); d = d.AddDate(0, 0, 1) {
		date := practice.Date{Time: d}
		if closed[date.String()] {
			continue
		}
		day := c.day(date)
		wd := isoWeekday(day)
		open, ok := opening[wd]
		if !ok {
			continue
		}
		for _, p := range b.candidate {
			plan := dayPlan{Day: day}
			for _, a := range avail {
				if a.PractitionerID != p.ID || a.Weekday != wd {
					continue
				}
				if w, ok := intersect(minutes{a.StartsMinute, a.EndsMinute}, minutes{open.Start, open.End}); ok {
					plan.Working = append(plan.Working, w)
				}
			}
			for _, t := range freeStarts(plan, c.tz, c.profile.SlotMinutes, b.service.DurationMinutes, busy[p.ID], earliest, latest) {
				sl, ok := byStart[t.Unix()]
				if !ok {
					sl = &Slot{StartsAt: t.UTC(), Local: c.local(t).Format(localLayout), Label: c.local(t).Format(labelLayout)}
					byStart[t.Unix()] = sl
				}
				sl.PractitionerIDs = append(sl.PractitionerIDs, p.ID) // candidates are sorted by name
			}
		}
	}
	out := make([]Slot, 0, len(byStart))
	for _, sl := range byStart {
		out = append(out, *sl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartsAt.Before(out[j].StartsAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// busy loads each practitioner's blocks and non-cancelled appointments
// overlapping [from, to), at any location.
func (s *Service) busy(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID, from, to time.Time, ignore *uuid.UUID) (map[uuid.UUID][]span, error) {
	out := map[uuid.UUID][]span{}
	var blocks []Block
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND practitioner_id IN ? AND starts_at < ? AND ends_at > ?", orgID, ids, to, from).
		Find(&blocks).Error; err != nil {
		return nil, fmt.Errorf("load blocks: %w", err)
	}
	for _, b := range blocks {
		out[b.PractitionerID] = append(out[b.PractitionerID], span{b.StartsAt, b.EndsAt})
	}
	q := s.db.WithContext(ctx).Where("organization_id = ? AND practitioner_id IN ? AND status <> ? AND starts_at < ? AND ends_at > ?",
		orgID, ids, StatusCancelled, to, from)
	if ignore != nil {
		q = q.Where("id <> ?", *ignore)
	}
	var appts []appointmentRow
	if err := q.Find(&appts).Error; err != nil {
		return nil, fmt.Errorf("load appointments: %w", err)
	}
	for _, a := range appts {
		out[a.PractitionerID] = append(out[a.PractitionerID], span{a.StartsAt, a.EndsAt})
	}
	return out, nil
}

// ---------- appointments (R7–R13) ----------

// NewAppointment is a booking request.
type NewAppointment struct {
	ContactID      uuid.UUID
	ServiceID      uuid.UUID
	LocationID     uuid.UUID
	PractitionerID uuid.UUID
	StartsAt       time.Time
	Note           string
	// OutsideHours lets staff book outside hours and availability (R9). Never
	// set for patients. Clashes are refused regardless.
	OutsideHours bool
	// AllowPatientOverlap lets staff book a patient twice at once (R13).
	AllowPatientOverlap bool
}

// Create books an appointment after re-checking the slot (R8, R9).
func (s *Service) Create(ctx context.Context, orgID uuid.UUID, actor Actor, in NewAppointment) (*Appointment, error) {
	c, err := s.clinic(ctx, orgID)
	if err != nil {
		return nil, err
	}
	patient := actor.Source != SourceStaff
	if patient && (in.OutsideHours || in.AllowPatientOverlap) {
		return nil, kit.Forbidden("only staff can book outside hours")
	}
	note, err := cleanNote(in.Note)
	if err != nil {
		return nil, err
	}
	if _, err := s.patient(ctx, orgID, in.ContactID); err != nil {
		return nil, err
	}
	b, err := s.bookable(ctx, orgID, in.ServiceID, in.LocationID, &in.PractitionerID)
	if err != nil {
		return nil, err
	}
	start := in.StartsAt.UTC()
	end := start.Add(time.Duration(b.service.DurationMinutes) * time.Minute)
	if err := s.checkTime(ctx, orgID, c, b, start, end, patient, in.OutsideHours, nil); err != nil {
		return nil, err
	}
	if err := s.checkPatientOverlap(ctx, orgID, c, in.ContactID, start, end, nil, patient || !in.AllowPatientOverlap); err != nil {
		return nil, err
	}

	row := appointmentRow{OrganizationID: orgID, ContactID: in.ContactID, PractitionerID: in.PractitionerID,
		ServiceID: in.ServiceID, LocationID: in.LocationID, StartsAt: start, EndsAt: end, Status: StatusBooked,
		Source: actor.Source, Note: note, OutsideHours: in.OutsideHours, CreatedByID: actor.UserID}
	for attempt := 0; ; attempt++ {
		row.ID = uuid.New()
		row.Reference = newReference()
		err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			return tx.Create(&historyRow{OrganizationID: orgID, AppointmentID: row.ID, ToStatus: StatusBooked,
				Changes: models.JSONB{}, ChangedByID: actor.UserID, Source: actor.Source}).Error
		})
		if isSQLState(err, "23505") && strings.Contains(err.Error(), "booking_appointments_org_ref") && attempt < 5 {
			continue // reference collision: draw another
		}
		break
	}
	if isSQLState(err, "23P01") {
		// Lost a race the check above didn't see: name the appointment that won.
		if clash := s.clash(ctx, orgID, c, b.candidate[0], start, end, nil); clash != nil {
			return nil, clash
		}
		return nil, kit.Conflict("that time was just taken")
	}
	if err != nil {
		return nil, fmt.Errorf("save appointment: %w", err)
	}
	return s.Get(ctx, orgID, row.ID)
}

// checkTime applies the time rules to [start, end) for the one candidate
// practitioner in b: grid, past, notice and window (patients), clashes (always,
// 409), then hours and availability unless staff override.
func (s *Service) checkTime(ctx context.Context, orgID uuid.UUID, c *clinic, b *bookable, start, end time.Time, patient, outsideHours bool, ignore *uuid.UUID) error {
	pr := b.candidate[0]
	local := c.local(start)
	if (local.Hour()*60+local.Minute())%c.profile.SlotMinutes != 0 || local.Second() != 0 || local.Nanosecond() != 0 {
		return kit.Invalid("the start time must be on the clinic's %d-minute grid", c.profile.SlotMinutes)
	}
	now := s.now()
	if start.Before(now) {
		return kit.Invalid("that time is in the past")
	}
	if patient {
		if start.Before(now.Add(time.Duration(c.settings.MinNoticeMinutes) * time.Minute)) {
			return kit.Invalid("bookings need at least %s notice", humanMinutes(c.settings.MinNoticeMinutes))
		}
		if start.After(now.AddDate(0, 0, c.settings.WindowDays)) {
			return kit.Invalid("bookings open %d days ahead", c.settings.WindowDays)
		}
	}
	if clash := s.clash(ctx, orgID, c, pr, start, end, ignore); clash != nil {
		return clash
	}
	if outsideHours {
		return nil
	}
	date := practice.Date{Time: time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)}
	pid := pr.ID
	slots, err := s.searchSlots(ctx, orgID, c, SlotQuery{ServiceID: b.service.ID, LocationID: b.location.ID,
		PractitionerID: &pid, From: date, To: date, ignore: ignore})
	if err != nil {
		return err
	}
	for _, sl := range slots {
		if sl.StartsAt.Equal(start) {
			return nil
		}
	}
	// Someone may have booked it since the clash check above: say so (409)
	// rather than calling the time unavailable (400).
	if clash := s.clash(ctx, orgID, c, pr, start, end, ignore); clash != nil {
		return clash
	}
	return kit.Invalid("%s isn't available at %s at %s", pr.DisplayName, b.location.Name, local.Format(labelLayout))
}

// clash returns a 409 naming an appointment of the practitioner that overlaps
// [start, end), or nil.
func (s *Service) clash(ctx context.Context, orgID uuid.UUID, c *clinic, pr practice.Practitioner, start, end time.Time, ignore *uuid.UUID) error {
	q := s.db.WithContext(ctx).Where("organization_id = ? AND practitioner_id = ? AND status <> ? AND starts_at < ? AND ends_at > ?",
		orgID, pr.ID, StatusCancelled, end, start)
	if ignore != nil {
		q = q.Where("id <> ?", *ignore)
	}
	var found []appointmentRow
	if err := q.Order("starts_at").Limit(1).Find(&found).Error; err != nil || len(found) == 0 {
		return nil
	}
	f := found[0]
	return kit.Conflict("%s already has an appointment %s–%s on %s", pr.DisplayName,
		c.local(f.StartsAt).Format("15:04"), c.local(f.EndsAt).Format("15:04"), c.local(f.StartsAt).Format("Mon 2 Jan"))
}

// checkPatientOverlap refuses (refuse=true) a second appointment for the same
// patient at the same time (R13).
func (s *Service) checkPatientOverlap(ctx context.Context, orgID uuid.UUID, c *clinic, contactID uuid.UUID, start, end time.Time, ignore *uuid.UUID, refuse bool) error {
	if !refuse {
		return nil
	}
	q := s.db.WithContext(ctx).Where("organization_id = ? AND contact_id = ? AND status <> ? AND starts_at < ? AND ends_at > ?",
		orgID, contactID, StatusCancelled, end, start)
	if ignore != nil {
		q = q.Where("id <> ?", *ignore)
	}
	var found []appointmentRow
	if err := q.Limit(1).Find(&found).Error; err != nil {
		return fmt.Errorf("check patient's appointments: %w", err)
	}
	if len(found) > 0 {
		return kit.Conflict("the patient already has an appointment at %s", c.local(found[0].StartsAt).Format(labelLayout))
	}
	return nil
}

// MoveInput changes an appointment's time, practitioner, location or note.
type MoveInput struct {
	StartsAt       *time.Time
	PractitionerID *uuid.UUID
	LocationID     *uuid.UUID
	Note           *string
	OutsideHours   *bool
}

// Move re-checks and moves a BOOKED or CONFIRMED appointment (R12) and records
// the change (R11).
func (s *Service) Move(ctx context.Context, orgID uuid.UUID, actor Actor, id uuid.UUID, in MoveInput) (*Appointment, error) {
	c, err := s.clinic(ctx, orgID)
	if err != nil {
		return nil, err
	}
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	patient := actor.Source != SourceStaff
	upd := *row
	changes := models.JSONB{}
	if in.Note != nil {
		note, err := cleanNote(*in.Note)
		if err != nil {
			return nil, err
		}
		if note != row.Note {
			upd.Note = note
			changes["note"] = true // the text itself isn't copied into history
		}
	}
	moved := (in.StartsAt != nil && !in.StartsAt.Equal(row.StartsAt)) ||
		(in.PractitionerID != nil && *in.PractitionerID != row.PractitionerID) ||
		(in.LocationID != nil && *in.LocationID != row.LocationID)
	if moved {
		if !row.Status.movable() {
			return nil, kit.Conflict("a %s appointment can't be moved", strings.ToLower(string(row.Status)))
		}
		if in.StartsAt != nil {
			upd.StartsAt = in.StartsAt.UTC()
		}
		if in.PractitionerID != nil {
			upd.PractitionerID = *in.PractitionerID
		}
		if in.LocationID != nil {
			upd.LocationID = *in.LocationID
		}
		upd.OutsideHours = in.OutsideHours != nil && *in.OutsideHours
		if patient && upd.OutsideHours {
			return nil, kit.Forbidden("only staff can book outside hours")
		}
		b, err := s.bookable(ctx, orgID, row.ServiceID, upd.LocationID, &upd.PractitionerID)
		if err != nil {
			return nil, err
		}
		upd.EndsAt = upd.StartsAt.Add(row.EndsAt.Sub(row.StartsAt))
		if err := s.checkTime(ctx, orgID, c, b, upd.StartsAt, upd.EndsAt, patient, upd.OutsideHours, &row.ID); err != nil {
			return nil, err
		}
		if err := s.checkPatientOverlap(ctx, orgID, c, row.ContactID, upd.StartsAt, upd.EndsAt, &row.ID, patient); err != nil {
			return nil, err
		}
		if !upd.StartsAt.Equal(row.StartsAt) {
			changes["starts_at"] = []any{row.StartsAt, upd.StartsAt}
		}
		if upd.PractitionerID != row.PractitionerID {
			changes["practitioner_id"] = []any{row.PractitionerID, upd.PractitionerID}
		}
		if upd.LocationID != row.LocationID {
			changes["location_id"] = []any{row.LocationID, upd.LocationID}
		}
		if upd.OutsideHours {
			changes["outside_hours"] = true
		}
	}
	if len(changes) == 0 {
		return s.Get(ctx, orgID, id)
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&appointmentRow{}).Where("organization_id = ? AND id = ?", orgID, id).Updates(map[string]any{
			"starts_at": upd.StartsAt, "ends_at": upd.EndsAt, "practitioner_id": upd.PractitionerID,
			"location_id": upd.LocationID, "note": upd.Note, "outside_hours": upd.OutsideHours, "updated_at": s.now(),
		}).Error; err != nil {
			return err
		}
		from := row.Status
		return tx.Create(&historyRow{OrganizationID: orgID, AppointmentID: id, FromStatus: &from, ToStatus: row.Status,
			Changes: changes, ChangedByID: actor.UserID, Source: actor.Source}).Error
	})
	if isSQLState(err, "23P01") {
		return nil, kit.Conflict("that time was just taken")
	}
	if err != nil {
		return nil, fmt.Errorf("move appointment: %w", err)
	}
	return s.Get(ctx, orgID, id)
}

// SetStatus moves an appointment through the status machine (R10) and records
// it (R11). Cancelling needs a reason and frees the time.
func (s *Service) SetStatus(ctx context.Context, orgID uuid.UUID, actor Actor, id uuid.UUID, to Status, reason string) (*Appointment, error) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > reasonMaxLen {
		return nil, kit.Invalid("reason is too long (max %d characters)", reasonMaxLen)
	}
	if to == StatusCancelled && reason == "" {
		return nil, kit.Invalid("a reason is required to cancel")
	}
	var row *appointmentRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var found []appointmentRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", orgID, id).
			Limit(1).Find(&found).Error; err != nil {
			return err
		}
		if len(found) == 0 {
			return kit.NotFound("appointment not found")
		}
		row = &found[0]
		if !row.Status.CanBecome(to) {
			return kit.Conflict("can't change %s to %s; allowed: %s", row.Status, to, joinStatuses(row.Status.Next()))
		}
		if err := tx.Model(&appointmentRow{}).Where("organization_id = ? AND id = ?", orgID, id).
			Updates(map[string]any{"status": to, "updated_at": s.now()}).Error; err != nil {
			return err
		}
		from := row.Status
		return tx.Create(&historyRow{OrganizationID: orgID, AppointmentID: id, FromStatus: &from, ToStatus: to,
			Changes: models.JSONB{}, ChangedByID: actor.UserID, Source: actor.Source, Reason: reason}).Error
	})
	var ke *kit.Error
	if errors.As(err, &ke) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("change status: %w", err)
	}
	return s.Get(ctx, orgID, id)
}

func joinStatuses(ss []Status) string {
	if len(ss) == 0 {
		return "none (final status)"
	}
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = string(s)
	}
	return strings.Join(parts, ", ")
}

func (s *Service) row(ctx context.Context, orgID, id uuid.UUID) (*appointmentRow, error) {
	var found []appointmentRow
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", orgID, id).Limit(1).Find(&found).Error; err != nil {
		return nil, fmt.Errorf("load appointment: %w", err)
	}
	if len(found) == 0 {
		return nil, kit.NotFound("appointment not found")
	}
	return &found[0], nil
}

// Get returns one appointment.
func (s *Service) Get(ctx context.Context, orgID, id uuid.UUID) (*Appointment, error) {
	row, err := s.row(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	out, err := s.present(ctx, orgID, []appointmentRow{*row})
	if err != nil {
		return nil, err
	}
	return &out[0], nil
}

// History returns an appointment's changes, oldest first.
func (s *Service) History(ctx context.Context, orgID, id uuid.UUID) ([]HistoryEntry, error) {
	if _, err := s.row(ctx, orgID, id); err != nil {
		return nil, err
	}
	var rows []historyRow
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND appointment_id = ?", orgID, id).Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	userIDs := []uuid.UUID{}
	for _, r := range rows {
		if r.ChangedByID != nil {
			userIDs = append(userIDs, *r.ChangedByID)
		}
	}
	names := map[uuid.UUID]string{}
	if len(userIDs) > 0 {
		var users []models.User
		if err := s.db.WithContext(ctx).Select("id", "full_name").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, fmt.Errorf("load users: %w", err)
		}
		for _, u := range users {
			names[u.ID] = u.FullName
		}
	}
	out := make([]HistoryEntry, 0, len(rows))
	for _, r := range rows {
		e := HistoryEntry{FromStatus: r.FromStatus, ToStatus: r.ToStatus, Changes: map[string]any(r.Changes),
			Source: r.Source, Reason: r.Reason, CreatedAt: r.CreatedAt}
		if r.ChangedByID != nil {
			e.ChangedBy = &Ref{ID: *r.ChangedByID, Name: names[*r.ChangedByID]}
		}
		out = append(out, e)
	}
	return out, nil
}

// ListQuery filters appointments. From and To are local dates.
type ListQuery struct {
	From, To       practice.Date
	PractitionerID *uuid.UUID
	LocationID     *uuid.UUID
	ContactID      *uuid.UUID
	Status         *Status

	rangeFrom, rangeTo *time.Time // instant range instead of dates (internal)
	activeOnly         bool       // skip cancelled
}

// List returns appointments starting in the range, oldest first.
func (s *Service) List(ctx context.Context, orgID uuid.UUID, q ListQuery) ([]Appointment, error) {
	var from, to time.Time
	if q.rangeFrom != nil {
		from, to = *q.rangeFrom, *q.rangeTo
	} else {
		c, err := s.clinic(ctx, orgID)
		if err != nil {
			return nil, err
		}
		if q.From.IsZero() {
			q.From = c.today(s.now())
		}
		if q.To.IsZero() {
			q.To = q.From
		}
		if q.To.Before(q.From.Time) || q.To.Sub(q.From.Time) > (maxRangeDays-1)*24*time.Hour {
			return nil, kit.Invalid("list at most %d days at a time", maxRangeDays)
		}
		from, to = c.day(q.From), c.day(q.To).AddDate(0, 0, 1)
	}
	db := s.db.WithContext(ctx).Where("organization_id = ? AND starts_at < ? AND ends_at > ?", orgID, to, from)
	if q.rangeFrom == nil {
		db = s.db.WithContext(ctx).Where("organization_id = ? AND starts_at >= ? AND starts_at < ?", orgID, from, to)
	}
	if q.PractitionerID != nil {
		db = db.Where("practitioner_id = ?", *q.PractitionerID)
	}
	if q.LocationID != nil {
		db = db.Where("location_id = ?", *q.LocationID)
	}
	if q.ContactID != nil {
		db = db.Where("contact_id = ?", *q.ContactID)
	}
	if q.Status != nil {
		db = db.Where("status = ?", *q.Status)
	}
	if q.activeOnly {
		db = db.Where("status <> ?", StatusCancelled)
	}
	var rows []appointmentRow
	if err := db.Order("starts_at, id").Limit(1000).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list appointments: %w", err)
	}
	return s.present(ctx, orgID, rows)
}

// present turns rows into API appointments, loading names in a few queries.
func (s *Service) present(ctx context.Context, orgID uuid.UUID, rows []appointmentRow) ([]Appointment, error) {
	out := make([]Appointment, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	c, err := s.clinic(ctx, orgID)
	if err != nil {
		return nil, err
	}
	prs, err := s.practice.ListPractitioners(ctx, orgID, true)
	if err != nil {
		return nil, err
	}
	svcs, err := s.practice.ListServices(ctx, orgID, true)
	if err != nil {
		return nil, err
	}
	locs, err := s.practice.ListLocations(ctx, orgID, true)
	if err != nil {
		return nil, err
	}
	prName, svcName, locName := map[uuid.UUID]string{}, map[uuid.UUID]string{}, map[uuid.UUID]string{}
	for _, p := range prs {
		prName[p.ID] = p.DisplayName
	}
	for _, sv := range svcs {
		svcName[sv.ID] = sv.Name
	}
	for _, l := range locs {
		locName[l.ID] = l.Name
	}
	contactIDs := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		contactIDs = append(contactIDs, r.ContactID)
	}
	patients, err := s.patients(ctx, orgID, contactIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		l := c.local(r.StartsAt)
		p := patients[r.ContactID]
		p.ID = r.ContactID
		out = append(out, Appointment{
			ID: r.ID, Reference: r.Reference, Status: r.Status, Source: r.Source,
			StartsAt: r.StartsAt.UTC(), EndsAt: r.EndsAt.UTC(), Local: l.Format(localLayout), Label: l.Format(labelLayout),
			Contact:      p,
			Practitioner: Ref{ID: r.PractitionerID, Name: prName[r.PractitionerID]},
			Service:      Ref{ID: r.ServiceID, Name: svcName[r.ServiceID]},
			Duration:     int(r.EndsAt.Sub(r.StartsAt) / time.Minute),
			Location:     Ref{ID: r.LocationID, Name: locName[r.LocationID]},
			Note:         r.Note, OutsideHours: r.OutsideHours, CreatedAt: r.CreatedAt, AllowedNext: r.Status.Next(),
		})
	}
	return out, nil
}

// ---------- helpers ----------

func cleanNote(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > noteMaxLen {
		return "", kit.Invalid("note is too long (max %d characters)", noteMaxLen)
	}
	return s, nil
}

// referenceAlphabet leaves out look-alikes (0/O, 1/I/L).
const referenceAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newReference() string {
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(referenceAlphabet))))
		if err != nil {
			panic(err) // crypto/rand never fails on supported platforms
		}
		b[i] = referenceAlphabet[n.Int64()]
	}
	return string(b)
}

func formatMinute(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

func parseMinute(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		if s == "24:00" {
			return 1440, nil
		}
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

func weekdayName(d int) string {
	return [...]string{"", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}[d]
}

func humanMinutes(m int) string {
	switch {
	case m%1440 == 0:
		return plural(m/1440, "day")
	case m%60 == 0:
		return plural(m/60, "hour")
	default:
		return plural(m, "minute")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
