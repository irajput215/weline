package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/crypto"
	"github.com/shridarpatil/whatomate/internal/modules/audit"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Statuses of a connection.
const (
	StatusConnected = "connected"
	StatusError     = "error"
	StatusNotSetUp  = "not_set_up"

	keepSyncs   = 100 // R9, Q3
	maxErrorLen = 300
	testTimeout = 10 * time.Second
)

// Logger is the part of Weline's logger the module uses.
type Logger interface {
	Info(msg string, fields ...any)
	Error(msg string, fields ...any)
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

// Service holds the connector rules. Every method takes the organization first
// and filters every query by it.
type Service struct {
	db        *gorm.DB
	key       string // Weline's encryption_key; credentials aren't stored without it
	factories factories
	log       Logger
	now       func() time.Time
}

// NewService returns the connectors service with the given providers.
func NewService(db *gorm.DB, encryptionKey string, log Logger, fs ...Factory) *Service {
	if log == nil {
		log = nopLogger{}
	}
	s := &Service{db: db, key: encryptionKey, factories: factories{}, log: log, now: time.Now}
	for _, f := range fs {
		s.factories[f.Name] = f
	}
	return s
}

// ---------- rows ----------

type accountRow struct {
	OrganizationID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	Provider         string
	Credentials      string
	CredentialsSetAt *time.Time
	Status           string
	Settings         json.RawMessage `gorm:"type:jsonb"`
	LastTestAt       *time.Time
	LastSyncAt       *time.Time
	LastError        string
	UpdatedByID      *uuid.UUID `gorm:"type:uuid"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (accountRow) TableName() string { return "wl.connector_accounts" }

type syncRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	Provider       string
	Kind           string
	StartedAt      time.Time
	FinishedAt     *time.Time
	Outcome        string
	Counts         json.RawMessage `gorm:"type:jsonb"`
	Error          string
}

func (syncRow) TableName() string { return "wl.connector_syncs" }

type linkRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	Provider       string
	Entity         string
	LocalID        *uuid.UUID `gorm:"type:uuid"`
	RemoteID       string
	KeyHash        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (linkRow) TableName() string { return "wl.connector_links" }

// ---------- views ----------

// Account is the organization's connection as the API shows it. Credentials
// are never included, only whether and when they were set (R6).
type Account struct {
	Provider         string          `json:"provider"`
	Status           string          `json:"status"`
	CredentialsSet   bool            `json:"credentials_set"`
	CredentialsSetAt *time.Time      `json:"credentials_set_at"`
	Settings         json.RawMessage `json:"settings"`
	LastTestAt       *time.Time      `json:"last_test_at"`
	LastSyncAt       *time.Time      `json:"last_sync_at"`
	LastError        string          `json:"last_error"`
}

// ProviderInfo describes a provider for the settings screen.
type ProviderInfo struct {
	Name      string  `json:"name"`
	Available bool    `json:"available"`
	Fields    []Field `json:"fields"`
	ShareWith string  `json:"share_with,omitempty"` // an address the business shares its calendar with (Google)
}

// Sync is one log entry.
type Sync struct {
	ID         uuid.UUID       `json:"id"`
	Provider   string          `json:"provider"`
	Kind       string          `json:"kind"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
	Outcome    string          `json:"outcome"`
	Counts     json.RawMessage `json:"counts"`
	Error      string          `json:"error"`
}

func builtinAccount() *Account {
	return &Account{Provider: provider.BuiltIn, Status: StatusConnected, Settings: json.RawMessage("{}")}
}

func (r *accountRow) view() *Account {
	set := json.RawMessage("{}")
	if len(r.Settings) > 0 {
		set = r.Settings
	}
	return &Account{Provider: r.Provider, Status: r.Status, CredentialsSet: r.Credentials != "", CredentialsSetAt: r.CredentialsSetAt,
		Settings: set, LastTestAt: r.LastTestAt, LastSyncAt: r.LastSyncAt, LastError: r.LastError}
}

func (s *Service) row(ctx context.Context, orgID uuid.UUID) (*accountRow, error) {
	var rows []accountRow
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load connection: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// Account returns the organization's connection; built-in when there's none.
func (s *Service) Account(ctx context.Context, orgID uuid.UUID) (*Account, error) {
	r, err := s.row(ctx, orgID)
	if err != nil || r == nil {
		return builtinAccount(), err
	}
	return r.view(), nil
}

// Providers lists the providers for the settings screen: those built into
// this release, then those coming soon.
func (s *Service) Providers() []ProviderInfo {
	out := []ProviderInfo{}
	for _, f := range s.factories.list() {
		fields := f.Fields
		if fields == nil {
			fields = []Field{}
		}
		out = append(out, ProviderInfo{Name: f.Name, Available: f.Available, Fields: fields, ShareWith: f.ShareWith})
	}
	for _, f := range comingSoon {
		if _, ok := s.factories[f.Name]; !ok {
			out = append(out, ProviderInfo{Name: f.Name, Available: false, Fields: f.Fields})
		}
	}
	return out
}

// ---------- choosing a provider ----------

// SaveInput chooses a provider. Credentials may be omitted to keep the stored
// ones when the provider doesn't change.
type SaveInput struct {
	Provider    string            `json:"provider"`
	Credentials map[string]string `json:"credentials"`
	Settings    json.RawMessage   `json:"settings"`
}

// safeMessage is an error's message as safe to store and show: a kit error's
// message, or a generic one; never longer than the column.
func safeMessage(err error) string {
	var e *kit.Error
	msg := "the connection test failed"
	if errors.As(err, &e) {
		msg = e.Message
	}
	if len([]rune(msg)) > maxErrorLen {
		msg = string([]rune(msg)[:maxErrorLen])
	}
	return msg
}

// Save chooses the organization's provider (R5–R7). Another provider than
// built-in is saved only after its connection test passes.
func (s *Service) Save(ctx context.Context, orgID, userID uuid.UUID, in SaveInput) (*Account, error) {
	in.Provider = strings.TrimSpace(in.Provider)
	f, ok := s.factories[in.Provider]
	if !ok || !f.Available {
		return nil, kit.Invalid("provider: %q isn't available", in.Provider)
	}
	settings := in.Settings
	if len(settings) == 0 || string(settings) == "null" {
		settings = json.RawMessage("{}")
	}
	var obj map[string]any
	if err := json.Unmarshal(settings, &obj); err != nil {
		return nil, kit.Invalid("settings: must be an object")
	}
	now := s.now()
	row := accountRow{OrganizationID: orgID, Provider: in.Provider, Status: StatusConnected, Settings: settings,
		UpdatedByID: &userID, CreatedAt: now, UpdatedAt: now}

	if in.Provider != provider.BuiltIn {
		creds := in.Credentials
		cur, err := s.row(ctx, orgID)
		if err != nil {
			return nil, err
		}
		if len(creds) == 0 && cur != nil && cur.Provider == in.Provider && cur.Credentials != "" {
			if creds, err = s.decrypt(cur.Credentials); err != nil {
				return nil, err
			}
			row.CredentialsSetAt = cur.CredentialsSetAt
		} else {
			row.CredentialsSetAt = &now
		}
		for _, fl := range f.Fields {
			if strings.TrimSpace(creds[fl.Name]) == "" {
				return nil, kit.Invalid("credentials: %s is required", fl.Name)
			}
		}
		if s.key == "" {
			return nil, kit.NotReady("credentials can't be stored safely: Weline's encryption key isn't set (WHATOMATE_APP__ENCRYPTION_KEY)")
		}
		msg, err := s.test(ctx, orgID, f, creds, settings)
		if err != nil {
			return nil, kit.Invalid("connection test failed: %s", msg)
		}
		if err := s.claim(ctx, orgID, f, creds, settings); err != nil {
			return nil, err
		}
		enc, err := s.encrypt(creds)
		if err != nil {
			return nil, err
		}
		row.Credentials = enc
		row.LastTestAt = &now
	} else if err := s.claim(ctx, orgID, f, nil, settings); err != nil { // releases any calendars held
		return nil, err
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "organization_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"provider", "credentials", "credentials_set_at", "status", "settings",
			"last_test_at", "last_error", "updated_by_id", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("save connection: %w", err)
	}
	audit.Log(ctx, s.db, s.log, orgID, audit.Entry{Actor: &userID, Module: Name, Action: "connection_saved", Entity: "connection",
		Details: map[string]any{"provider": in.Provider, "new_credentials_set": len(in.Credentials) > 0}})
	return s.Account(ctx, orgID)
}

// test runs a provider's connection test with a timeout and logs it.
func (s *Service) test(ctx context.Context, orgID uuid.UUID, f Factory, creds map[string]string, settings json.RawMessage) (string, error) {
	start := s.now()
	if f.Test == nil {
		s.logSync(ctx, orgID, f.Name, "test", start, nil, "")
		return "connected", nil
	}
	tctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	msg, err := f.Test(tctx, creds, settings)
	if err != nil {
		msg = safeMessage(err)
		s.logSync(ctx, orgID, f.Name, "test", start, nil, msg)
		return msg, err
	}
	s.logSync(ctx, orgID, f.Name, "test", start, nil, "")
	return msg, nil
}

// TestResult is what re-testing the stored connection found.
type TestResult struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Account *Account `json:"account"`
}

// Test re-tests the stored connection and records the result.
func (s *Service) Test(ctx context.Context, orgID, userID uuid.UUID) (*TestResult, error) {
	r, err := s.row(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if r == nil || r.Provider == provider.BuiltIn {
		return &TestResult{OK: true, Message: "Weline's built-in booking is always connected", Account: builtinAccountOr(r)}, nil
	}
	f, ok := s.factories[r.Provider]
	if !ok {
		return nil, kit.Conflict("%s isn't available in this version of Weline", r.Provider)
	}
	creds, err := s.decrypt(r.Credentials)
	if err != nil {
		return nil, err
	}
	msg, terr := s.test(ctx, orgID, f, creds, r.Settings)
	now := s.now()
	upd := map[string]any{"last_test_at": now, "updated_at": now, "status": StatusConnected, "last_error": ""}
	if terr != nil {
		upd["status"], upd["last_error"] = StatusError, msg
	}
	if err := s.db.WithContext(ctx).Model(&accountRow{}).Where("organization_id = ?", orgID).Updates(upd).Error; err != nil {
		return nil, fmt.Errorf("save test result: %w", err)
	}
	audit.Log(ctx, s.db, s.log, orgID, audit.Entry{Actor: &userID, Module: Name, Action: "connection_tested", Entity: "connection",
		Details: map[string]any{"provider": r.Provider, "ok": terr == nil}})
	acc, err := s.Account(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return &TestResult{OK: terr == nil, Message: msg, Account: acc}, nil
}

func builtinAccountOr(r *accountRow) *Account {
	if r == nil {
		return builtinAccount()
	}
	return r.view()
}

func (s *Service) encrypt(creds map[string]string) (string, error) {
	b, err := json.Marshal(creds)
	if err != nil {
		return "", fmt.Errorf("encode credentials: %w", err)
	}
	enc, err := crypto.Encrypt(string(b), s.key)
	if err != nil {
		return "", fmt.Errorf("encrypt credentials: %w", err)
	}
	return enc, nil
}

func (s *Service) decrypt(enc string) (map[string]string, error) {
	if enc == "" {
		return map[string]string{}, nil
	}
	plain, err := crypto.Decrypt(enc, s.key)
	if err != nil {
		return nil, fmt.Errorf("decrypt credentials: %w", err)
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(plain), &out); err != nil {
		return nil, fmt.Errorf("decode credentials: %w", err)
	}
	return out, nil
}

// ---------- the resolver ----------

// For implements provider.Resolver: the organization's active provider, built
// with its credentials and bound to it (R4, R5).
func (s *Service) For(ctx context.Context, orgID uuid.UUID) (provider.Provider, error) {
	r, err := s.row(ctx, orgID)
	if err != nil {
		return nil, err
	}
	name := provider.BuiltIn
	var creds map[string]string
	var settings json.RawMessage
	if r != nil {
		name, settings = r.Provider, r.Settings
		if creds, err = s.decrypt(r.Credentials); err != nil {
			return nil, err
		}
	}
	f, ok := s.factories[name]
	if !ok || f.Build == nil {
		return nil, provider.ErrNotConnected
	}
	return f.Build(orgID, creds, settings)
}

// ---------- sync log ----------

// logSync records a test or sync run and keeps the last 100 (R9). errMsg is
// "" for success, otherwise a message safe to show (no patient data). It
// never fails the caller: a log write error is logged and dropped.
func (s *Service) logSync(ctx context.Context, orgID uuid.UUID, prov, kind string, start time.Time, counts map[string]int, errMsg string) {
	now := s.now()
	row := syncRow{OrganizationID: orgID, Provider: prov, Kind: kind, StartedAt: start, FinishedAt: &now, Outcome: "ok",
		Counts: json.RawMessage("{}")}
	if counts != nil {
		if b, err := json.Marshal(counts); err == nil {
			row.Counts = b
		}
	}
	if errMsg != "" {
		row.Outcome, row.Error = "error", errMsg
	}
	db := s.db.WithContext(ctx)
	if err := db.Create(&row).Error; err != nil {
		s.log.Error("connectors: sync log not written", "organization_id", orgID, "error", err)
		return
	}
	if err := db.Exec(`DELETE FROM wl.connector_syncs WHERE organization_id = ? AND id NOT IN (
		SELECT id FROM wl.connector_syncs WHERE organization_id = ? ORDER BY started_at DESC, id DESC LIMIT ?)`,
		orgID, orgID, keepSyncs).Error; err != nil {
		s.log.Error("connectors: sync log not trimmed", "organization_id", orgID, "error", err)
	}
}

// RecordSync is for providers' sync jobs (3.2): it logs the run and, when it
// succeeded, moves last_sync_at.
func (s *Service) RecordSync(ctx context.Context, orgID uuid.UUID, prov string, start time.Time, counts map[string]int, runErr error) {
	msg := ""
	if runErr != nil {
		msg = safeMessage(runErr)
	}
	s.logSync(ctx, orgID, prov, "sync", start, counts, msg)
	upd := map[string]any{"last_sync_at": s.now(), "last_error": "", "status": StatusConnected}
	if runErr != nil {
		upd = map[string]any{"last_error": msg, "status": StatusError}
	}
	if err := s.db.WithContext(ctx).Model(&accountRow{}).Where("organization_id = ? AND provider = ?", orgID, prov).
		Updates(upd).Error; err != nil {
		s.log.Error("connectors: sync status not saved", "organization_id", orgID, "error", err)
	}
}

// Syncs returns the log, newest first.
func (s *Service) Syncs(ctx context.Context, orgID uuid.UUID) ([]Sync, error) {
	var rows []syncRow
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Order("started_at DESC, id DESC").Limit(keepSyncs).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load sync log: %w", err)
	}
	out := make([]Sync, 0, len(rows))
	for _, r := range rows {
		out = append(out, Sync{ID: r.ID, Provider: r.Provider, Kind: r.Kind, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
			Outcome: r.Outcome, Counts: r.Counts, Error: r.Error})
	}
	return out, nil
}

// ---------- links (R8) ----------

// Links maps Weline records to a provider's, for one organization and
// provider. Providers get one from the module when they're built (3.2).
type Links struct {
	db       *gorm.DB
	orgID    uuid.UUID
	provider string
}

// Links returns the link store for an organization and provider.
func (s *Service) Links(orgID uuid.UUID, prov string) *Links {
	return &Links{db: s.db, orgID: orgID, provider: prov}
}

// Set links a Weline record (contact or appointment) to a provider id,
// replacing an earlier link of the same record.
func (l *Links) Set(ctx context.Context, entity string, localID uuid.UUID, remoteID string) error {
	now := time.Now()
	row := linkRow{OrganizationID: l.orgID, Provider: l.provider, Entity: entity, LocalID: &localID, RemoteID: remoteID,
		CreatedAt: now, UpdatedAt: now}
	err := l.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "organization_id"}, {Name: "provider"}, {Name: "entity"}, {Name: "local_id"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "local_id IS NOT NULL"}}},
		DoUpdates:   clause.AssignmentColumns([]string{"remote_id", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("save link: %w", err)
	}
	return nil
}

// Remote returns the provider id linked to a Weline record, or "".
func (l *Links) Remote(ctx context.Context, entity string, localID uuid.UUID) (string, error) {
	var ids []string
	if err := l.db.WithContext(ctx).Model(&linkRow{}).Where("organization_id = ? AND provider = ? AND entity = ? AND local_id = ?",
		l.orgID, l.provider, entity, localID).Limit(1).Pluck("remote_id", &ids).Error; err != nil {
		return "", fmt.Errorf("load link: %w", err)
	}
	if len(ids) == 0 {
		return "", nil
	}
	return ids[0], nil
}

// Local returns the Weline record linked to a provider id, or uuid.Nil.
func (l *Links) Local(ctx context.Context, entity, remoteID string) (uuid.UUID, error) {
	var rows []linkRow
	if err := l.db.WithContext(ctx).Where("organization_id = ? AND provider = ? AND entity = ? AND remote_id = ?",
		l.orgID, l.provider, entity, remoteID).Limit(1).Find(&rows).Error; err != nil {
		return uuid.Nil, fmt.Errorf("load link: %w", err)
	}
	if len(rows) == 0 || rows[0].LocalID == nil {
		return uuid.Nil, nil
	}
	return *rows[0].LocalID, nil
}

// PhoneHash is the hash a phone link stores: SHA-256 of the digits, with the
// organization as salt so hashes can't be compared across organizations.
func PhoneHash(orgID uuid.UUID, phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	sum := sha256.Sum256([]byte(orgID.String() + ":" + b.String()))
	return hex.EncodeToString(sum[:])
}

// SetPhone records that a provider record has this mobile (stored as a hash).
func (l *Links) SetPhone(ctx context.Context, phone, remoteID string) error {
	h := PhoneHash(l.orgID, phone)
	var n int64
	if err := l.db.WithContext(ctx).Model(&linkRow{}).Where("organization_id = ? AND provider = ? AND entity = 'phone' AND key_hash = ? AND remote_id = ?",
		l.orgID, l.provider, h, remoteID).Count(&n).Error; err != nil {
		return fmt.Errorf("check phone link: %w", err)
	}
	if n > 0 {
		return nil
	}
	now := time.Now()
	if err := l.db.WithContext(ctx).Create(&linkRow{OrganizationID: l.orgID, Provider: l.provider, Entity: "phone",
		RemoteID: remoteID, KeyHash: h, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		return fmt.Errorf("save phone link: %w", err)
	}
	return nil
}

// ByPhone returns the provider records with this mobile, most recent first.
func (l *Links) ByPhone(ctx context.Context, phone string) ([]string, error) {
	var ids []string
	if err := l.db.WithContext(ctx).Model(&linkRow{}).Where("organization_id = ? AND provider = ? AND entity = 'phone' AND key_hash = ?",
		l.orgID, l.provider, PhoneHash(l.orgID, phone)).Order("updated_at DESC").Pluck("remote_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("load phone links: %w", err)
	}
	return ids, nil
}

var errClaimed = kit.Conflict("this calendar is already connected to another Weline account; if it's yours, contact Weline support")

func claimHash(prov, key string) string {
	h := sha256.Sum256([]byte(prov + ":" + strings.ToLower(strings.TrimSpace(key))))
	return hex.EncodeToString(h[:])
}

// claim records the provider's shared resources as this organization's,
// refusing any another organization holds, and releases ones it no longer uses.
func (s *Service) claim(ctx context.Context, orgID uuid.UUID, f Factory, creds map[string]string, settings json.RawMessage) error {
	var keys []string
	if f.claims != nil {
		var err error
		if keys, err = f.claims(creds, settings); err != nil {
			return err
		}
	}
	hashes := make([]string, 0, len(keys))
	for _, k := range keys {
		hashes = append(hashes, claimHash(f.Name, k))
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, h := range hashes {
			if err := tx.Exec(`INSERT INTO wl.connector_claims (provider, key_hash, organization_id) VALUES (?, ?, ?)
				ON CONFLICT (provider, key_hash) DO NOTHING`, f.Name, h, orgID).Error; err != nil {
				return err
			}
			var owner struct{ OrganizationID uuid.UUID }
			if err := tx.Raw(`SELECT organization_id FROM wl.connector_claims WHERE provider = ? AND key_hash = ?`, f.Name, h).Scan(&owner).Error; err != nil {
				return err
			}
			if owner.OrganizationID != orgID {
				return errClaimed
			}
		}
		q := tx.Where("organization_id = ?", orgID)
		if len(hashes) > 0 {
			q = q.Where("NOT (provider = ? AND key_hash IN ?)", f.Name, hashes)
		}
		return q.Delete(&claimRow{}).Error
	})
}

type claimRow struct {
	Provider       string    `gorm:"primaryKey"`
	KeyHash        string    `gorm:"primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
}

func (claimRow) TableName() string { return "wl.connector_claims" }
