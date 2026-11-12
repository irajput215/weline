package consent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

const (
	maxKeywords    = 30
	maxKeywordLen  = 50
	maxReplyLen    = 500
	maxTagLen      = 50
	maxMatchLength = 60 // longer messages are never keywords
	settingsTTL    = time.Minute

	ServiceContinue = "continue" // after STOP, service messages keep coming (R10, default)
	ServiceStop     = "stop"     // after STOP, no messages except replies
)

// Settings are an organization's consent settings (R1, R3, R5, R9, R10, R13).
type Settings struct {
	OptOutKeywords   []string `json:"opt_out_keywords"`
	OptInKeywords    []string `json:"opt_in_keywords"`
	OptInPhrases     []string `json:"opt_in_phrases"`
	OptOutReply      string   `json:"opt_out_reply"`
	OptInReply       string   `json:"opt_in_reply"`
	OptOutTag        string   `json:"opt_out_tag"`
	RequireOptIn     bool     `json:"require_opt_in"`
	ServiceAfterStop string   `json:"service_after_stop"`
}

// Defaults are used until an organization saves its own settings.
func Defaults() Settings {
	return Settings{
		OptOutKeywords: []string{"STOP", "STOP ALL", "STOP MESSAGES", "UNSUBSCRIBE", "OPT OUT", "OPTOUT"},
		OptInKeywords:  []string{"START", "SUBSCRIBE", "YES PLEASE"},
		OptInPhrases:   []string{},
		OptOutReply: "You won't get offers or news from {business} on WhatsApp any more. Reply START to get them again. " +
			"Appointment messages still come.",
		OptInReply:       "Thanks! You'll get offers and news from {business} on WhatsApp. Reply STOP at any time to stop them.",
		OptOutTag:        "opted-out",
		ServiceAfterStop: ServiceContinue,
	}
}

// settingsRow is wl.consent_settings.
type settingsRow struct {
	OrganizationID   uuid.UUID          `gorm:"type:uuid;primaryKey"`
	OptOutKeywords   models.StringArray `gorm:"type:jsonb"`
	OptInKeywords    models.StringArray `gorm:"type:jsonb"`
	OptInPhrases     models.StringArray `gorm:"type:jsonb"`
	OptOutReply      string
	OptInReply       string
	OptOutTag        string
	RequireOptIn     bool
	ServiceAfterStop string
	UpdatedByID      *uuid.UUID `gorm:"type:uuid"`
	UpdatedAt        time.Time
}

func (settingsRow) TableName() string { return "wl.consent_settings" }

// normalise prepares text for keyword matching: anything but letters and
// digits becomes a space, spaces collapse, upper case (design.md, "Keyword matching").
func normalise(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToUpper(r))
		default: // spaces, punctuation and emoji separate words: "opt-out" = "OPT OUT"
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func cleanList(field string, in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, k := range in {
		n := normalise(k)
		if n == "" || seen[n] {
			continue
		}
		if len([]rune(n)) > maxKeywordLen {
			return nil, kit.Invalid("%s: %q is longer than %d characters", field, k, maxKeywordLen)
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) > maxKeywords {
		return nil, kit.Invalid("%s: at most %d entries", field, maxKeywords)
	}
	return out, nil
}

// validate normalises and checks settings before saving.
func (st *Settings) validate() error {
	var err error
	if st.OptOutKeywords, err = cleanList("opt_out_keywords", st.OptOutKeywords); err != nil {
		return err
	}
	if len(st.OptOutKeywords) == 0 {
		return kit.Invalid("opt_out_keywords: keep at least one opt-out keyword")
	}
	if st.OptInKeywords, err = cleanList("opt_in_keywords", st.OptInKeywords); err != nil {
		return err
	}
	if st.OptInPhrases, err = cleanList("opt_in_phrases", st.OptInPhrases); err != nil {
		return err
	}
	out := map[string]bool{}
	for _, k := range st.OptOutKeywords {
		out[k] = true
	}
	for _, k := range append(append([]string{}, st.OptInKeywords...), st.OptInPhrases...) {
		if out[k] {
			return kit.Invalid("opt_in_keywords: %q is also an opt-out keyword", k)
		}
	}
	st.OptOutReply, st.OptInReply = strings.TrimSpace(st.OptOutReply), strings.TrimSpace(st.OptInReply)
	for field, v := range map[string]string{"opt_out_reply": st.OptOutReply, "opt_in_reply": st.OptInReply} {
		if v == "" {
			return kit.Invalid("%s: write the reply", field)
		}
		if len([]rune(v)) > maxReplyLen {
			return kit.Invalid("%s: at most %d characters", field, maxReplyLen)
		}
	}
	st.OptOutTag = strings.TrimSpace(st.OptOutTag)
	if st.OptOutTag == "" || len([]rune(st.OptOutTag)) > maxTagLen {
		return kit.Invalid("opt_out_tag: 1 to %d characters", maxTagLen)
	}
	if st.ServiceAfterStop != ServiceContinue && st.ServiceAfterStop != ServiceStop {
		return kit.Invalid("service_after_stop: must be %s or %s", ServiceContinue, ServiceStop)
	}
	return nil
}

// SettingsPatch changes only the fields that are set.
type SettingsPatch struct {
	OptOutKeywords   *[]string `json:"opt_out_keywords"`
	OptInKeywords    *[]string `json:"opt_in_keywords"`
	OptInPhrases     *[]string `json:"opt_in_phrases"`
	OptOutReply      *string   `json:"opt_out_reply"`
	OptInReply       *string   `json:"opt_in_reply"`
	OptOutTag        *string   `json:"opt_out_tag"`
	RequireOptIn     *bool     `json:"require_opt_in"`
	ServiceAfterStop *string   `json:"service_after_stop"`
}

func (p SettingsPatch) apply(st *Settings) {
	if p.OptOutKeywords != nil {
		st.OptOutKeywords = *p.OptOutKeywords
	}
	if p.OptInKeywords != nil {
		st.OptInKeywords = *p.OptInKeywords
	}
	if p.OptInPhrases != nil {
		st.OptInPhrases = *p.OptInPhrases
	}
	if p.OptOutReply != nil {
		st.OptOutReply = *p.OptOutReply
	}
	if p.OptInReply != nil {
		st.OptInReply = *p.OptInReply
	}
	if p.OptOutTag != nil {
		st.OptOutTag = *p.OptOutTag
	}
	if p.RequireOptIn != nil {
		st.RequireOptIn = *p.RequireOptIn
	}
	if p.ServiceAfterStop != nil {
		st.ServiceAfterStop = *p.ServiceAfterStop
	}
}

type cachedSettings struct {
	st  Settings
	exp time.Time
}

// settingsCache keeps each organization's settings for a minute so event
// handling stays quick; saving clears the entry.
type settingsCache struct {
	mu sync.Mutex
	m  map[uuid.UUID]cachedSettings
}

func (c *settingsCache) get(org uuid.UUID) (Settings, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[org]
	if !ok || time.Now().After(e.exp) {
		return Settings{}, false
	}
	return e.st, true
}

func (c *settingsCache) put(org uuid.UUID, st Settings) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[uuid.UUID]cachedSettings{}
	}
	c.m[org] = cachedSettings{st: st, exp: time.Now().Add(settingsTTL)}
}

func (c *settingsCache) drop(org uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, org)
}

// cachedSettings returns the settings from a one-minute cache, for the event
// path only (every incoming message reads them). Saving through this service
// clears the entry.
func (s *Service) cachedSettings(ctx context.Context, orgID uuid.UUID) (Settings, error) {
	if st, ok := s.cache.get(orgID); ok {
		return st, nil
	}
	st, err := s.Settings(ctx, orgID)
	if err != nil {
		return Settings{}, err
	}
	s.cache.put(orgID, st)
	return st, nil
}

// Settings returns the organization's settings, or the defaults, always fresh:
// other modules (Audiences) read them through their own Service.
func (s *Service) Settings(ctx context.Context, orgID uuid.UUID) (Settings, error) {
	var rows []settingsRow
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Limit(1).Find(&rows).Error; err != nil {
		return Settings{}, fmt.Errorf("load consent settings: %w", err)
	}
	st := Defaults()
	if len(rows) == 1 {
		r := rows[0]
		st = Settings{OptOutKeywords: r.OptOutKeywords, OptInKeywords: r.OptInKeywords, OptInPhrases: r.OptInPhrases,
			OptOutReply: r.OptOutReply, OptInReply: r.OptInReply, OptOutTag: r.OptOutTag, RequireOptIn: r.RequireOptIn,
			ServiceAfterStop: r.ServiceAfterStop}
		for _, l := range []*[]string{&st.OptOutKeywords, &st.OptInKeywords, &st.OptInPhrases} {
			if *l == nil {
				*l = []string{}
			}
		}
	}
	return st, nil
}

// SaveSettings changes the organization's settings.
func (s *Service) SaveSettings(ctx context.Context, orgID, userID uuid.UUID, p SettingsPatch) (Settings, error) {
	st, err := s.Settings(ctx, orgID)
	if err != nil {
		return Settings{}, err
	}
	p.apply(&st)
	if err := st.validate(); err != nil {
		return Settings{}, err
	}
	row := settingsRow{OrganizationID: orgID, OptOutKeywords: st.OptOutKeywords, OptInKeywords: st.OptInKeywords,
		OptInPhrases: st.OptInPhrases, OptOutReply: st.OptOutReply, OptInReply: st.OptInReply, OptOutTag: st.OptOutTag,
		RequireOptIn: st.RequireOptIn, ServiceAfterStop: st.ServiceAfterStop, UpdatedByID: &userID, UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return Settings{}, fmt.Errorf("save consent settings: %w", err)
	}
	s.cache.drop(orgID)
	return st, nil
}
