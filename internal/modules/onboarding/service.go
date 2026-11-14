package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/database"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Verticals a business can choose (R5). Phase 7 adds each pack's content.
var Verticals = []string{"allied_health", "dental", "medical", "beauty", "fitness", "real_estate", "other"}

const minPassword = 12 // upstream's rule for sign-up and login

// Service holds the onboarding rules. It talks to upstream tables only to
// create an organization the way upstream's "create organization" does, and to
// read what has been set up.
type Service struct {
	db    *gorm.DB
	grant func() error
	now   func() time.Time
}

// NewService builds the service; grant gives new organizations their module
// permissions (kit.GrantMissing through the registry).
func NewService(db *gorm.DB, grant func() error) *Service {
	return &Service{db: db, grant: grant, now: time.Now}
}

// SignupInput is the public sign-up form (R1).
type SignupInput struct {
	BusinessName string `json:"business_name"`
	FullName     string `json:"full_name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	TimeZone     string `json:"time_zone"`
	AcceptTerms  bool   `json:"accept_terms"`
	CaptchaToken string `json:"captcha_token"`
}

// SignupResult is what the page needs next.
type SignupResult struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	Email          string    `json:"email"`
}

var errEmailTaken = kit.Conflict("an account with this email already exists; sign in instead, or ask your organization's admin to add you")

func (in *SignupInput) clean() error {
	in.BusinessName = strings.Join(strings.Fields(in.BusinessName), " ")
	in.FullName = strings.Join(strings.Fields(in.FullName), " ")
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	switch n := utf8.RuneCountInString(in.BusinessName); {
	case n < 2 || n > 120:
		return kit.Invalid("business name must be 2 to 120 characters")
	}
	if n := utf8.RuneCountInString(in.FullName); n < 2 || n > 120 {
		return kit.Invalid("your name must be 2 to 120 characters")
	}
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email || len(in.Email) > 255 {
		return kit.Invalid("enter a valid email address")
	}
	if utf8.RuneCountInString(in.Password) < minPassword || len(in.Password) > 72 {
		return kit.Invalid("password must be at least %d characters (and at most 72 bytes)", minPassword)
	}
	if !in.AcceptTerms {
		return kit.Invalid("please accept the terms of service and privacy policy")
	}
	if in.TimeZone == "" {
		in.TimeZone = practice.DefaultTimeZone
	}
	return nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugFor(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 32 { // with the suffix, fits the booking page's 40 characters
		s = strings.Trim(s[:32], "-")
	}
	if len(s) < 2 {
		s = "business"
	}
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return s + "-" + hex.EncodeToString(b)
}

// Signup creates the organization, its system roles and chatbot settings (as
// upstream's "create organization" does), the first user as admin, the
// practice profile and the onboarding state, in one transaction; then grants
// module permissions (R1–R3).
func (s *Service) Signup(ctx context.Context, in SignupInput) (*SignupResult, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	var exists int64
	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("lower(email) = ?", in.Email).Count(&exists).Error; err != nil {
		return nil, err
	}
	if exists > 0 {
		return nil, errEmailTaken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	var res SignupResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		org := models.Organization{Name: in.BusinessName, Slug: slugFor(in.BusinessName), Settings: models.JSONB{}}
		if err := tx.Create(&org).Error; err != nil {
			return fmt.Errorf("create organization: %w", err)
		}
		if err := database.SeedSystemRolesForOrg(tx, org.ID); err != nil {
			return fmt.Errorf("seed roles: %w", err)
		}
		if err := tx.Create(&models.ChatbotSettings{OrganizationID: org.ID, IsEnabled: false, SessionTimeoutMins: 30}).Error; err != nil {
			return fmt.Errorf("chatbot settings: %w", err)
		}
		var admin models.CustomRole
		if err := tx.Where("organization_id = ? AND name = ? AND is_system", org.ID, "admin").First(&admin).Error; err != nil {
			return fmt.Errorf("admin role: %w", err)
		}
		user := models.User{OrganizationID: org.ID, Email: in.Email, PasswordHash: string(hash), FullName: in.FullName,
			RoleID: &admin.ID, IsActive: true, Settings: models.JSONB{}}
		if err := tx.Create(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "duplicate key") {
				return errEmailTaken
			}
			return fmt.Errorf("create user: %w", err)
		}
		if err := tx.Create(&models.UserOrganization{UserID: user.ID, OrganizationID: org.ID, RoleID: &admin.ID, IsDefault: true}).Error; err != nil {
			return fmt.Errorf("membership: %w", err)
		}
		if _, err := practice.NewService(tx).SaveProfile(ctx, org.ID, practice.ProfileInput{Name: in.BusinessName, TimeZone: in.TimeZone, BookingSlug: org.Slug}); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO wl.onboarding_state (organization_id, source, signed_up_by_id) VALUES (?, 'signup', ?)`, org.ID, user.ID).Error; err != nil {
			return fmt.Errorf("onboarding state: %w", err)
		}
		if err := audit.Record(ctx, tx, org.ID, audit.Entry{Actor: &user.ID, Module: Name, Action: "signed_up", Entity: "organization",
			EntityID: org.ID.String(), Details: map[string]any{"time_zone": in.TimeZone}}); err != nil {
			return err
		}
		res = SignupResult{OrganizationID: org.ID, Email: in.Email}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.grant != nil {
		if err := s.grant(); err != nil {
			return &res, fmt.Errorf("grant module permissions: %w", err)
		}
	}
	return &res, nil
}

// Step is one checklist item (R4).
type Step struct {
	Key  string `json:"key"`
	Done bool   `json:"done"`
	Link string `json:"link"` // the existing screen where it's done
}

// Checklist is the guided setup.
type Checklist struct {
	Vertical  string `json:"vertical"`
	Dismissed bool   `json:"dismissed"`
	Steps     []Step `json:"steps"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
}

type state struct {
	Vertical    string
	DismissedAt *time.Time
}

func (s *Service) state(ctx context.Context, orgID uuid.UUID) (state, error) {
	var st []state
	err := s.db.WithContext(ctx).Raw(`SELECT vertical, dismissed_at FROM wl.onboarding_state WHERE organization_id = ?`, orgID).Scan(&st).Error
	if err != nil || len(st) == 0 {
		return state{}, err
	}
	return st[0], nil
}

func (s *Service) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var ok bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS (`+query+`)`, args...).Scan(&ok).Error
	return ok, err
}

// Checklist works out each step from what the organization has set up.
func (s *Service) Checklist(ctx context.Context, orgID uuid.UUID) (*Checklist, error) {
	st, err := s.state(ctx, orgID)
	if err != nil {
		return nil, err
	}
	checks := []struct {
		key, link, query string
	}{
		{"business", "/x/practice/profile", `SELECT 1 FROM wl.practice_profiles WHERE organization_id = ?`},
		{"whatsapp", "/settings/accounts", `SELECT 1 FROM whatsapp_accounts WHERE organization_id = ? AND deleted_at IS NULL`},
		{"system", "/x/connectors", `SELECT 1 FROM wl.connector_accounts WHERE organization_id = ? AND status = 'connected'`},
		{"vertical", "/x/onboarding", `SELECT 1 FROM wl.onboarding_state WHERE organization_id = ? AND vertical <> ''`},
		{"team", "/settings/users", `SELECT 1 FROM user_organizations WHERE organization_id = ? AND deleted_at IS NULL OFFSET 1`},
		{"first_message", "/chat", `SELECT 1 FROM messages WHERE organization_id = ? AND direction = 'outgoing' AND deleted_at IS NULL`},
	}
	out := &Checklist{Vertical: st.Vertical, Dismissed: st.DismissedAt != nil, Total: len(checks)}
	for _, c := range checks {
		done, err := s.exists(ctx, c.query, orgID)
		if err != nil {
			return nil, fmt.Errorf("check %s: %w", c.key, err)
		}
		out.Steps = append(out.Steps, Step{Key: c.key, Done: done, Link: c.link})
		if done {
			out.Done++
		}
	}
	return out, nil
}

// SetVertical records the business's vertical (R5).
func (s *Service) SetVertical(ctx context.Context, orgID uuid.UUID, vertical string) error {
	ok := false
	for _, v := range Verticals {
		ok = ok || v == vertical
	}
	if !ok {
		return kit.Invalid("vertical must be one of %s", strings.Join(Verticals, ", "))
	}
	return s.upsert(ctx, orgID, "vertical", vertical)
}

// SetDismissed hides or shows the checklist (R6).
func (s *Service) SetDismissed(ctx context.Context, orgID uuid.UUID, dismissed bool) error {
	var v any
	if dismissed {
		v = s.now().UTC()
	}
	return s.upsert(ctx, orgID, "dismissed_at", v)
}

func (s *Service) upsert(ctx context.Context, orgID uuid.UUID, column string, value any) error {
	return s.db.WithContext(ctx).Exec(`INSERT INTO wl.onboarding_state (organization_id, `+column+`) VALUES (?, ?)
		ON CONFLICT (organization_id) DO UPDATE SET `+column+` = EXCLUDED.`+column+`, updated_at = now()`, orgID, value).Error
}
