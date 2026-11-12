package consent

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

const (
	maxImportBytes    = 1 << 20
	maxImportRows     = 5000
	maxImportEvidence = 1000
	importTTL         = 15 * time.Minute
	maxInvalidListed  = 50
)

// importRow is one parsed CSV line.
type importRow struct {
	Line    int       `json:"line"`
	Phone   string    `json:"phone"` // digits, country code first
	GivenAt time.Time `json:"given_at"`
	Note    string    `json:"note"`
}

// InvalidRow explains a rejected line.
type InvalidRow struct {
	Row    int    `json:"row"`
	Reason string `json:"reason"`
}

// ImportPreview is what applying would do (R7).
type ImportPreview struct {
	Token          string       `json:"token"`
	RowsTotal      int          `json:"rows_total"`
	WillApply      int          `json:"will_apply"`
	AlreadyOptedIn int          `json:"already_opted_in"`
	UnknownNumbers int          `json:"unknown_numbers"`
	Invalid        int          `json:"invalid"`
	InvalidRows    []InvalidRow `json:"invalid_rows"`
	OptedOutLater  int          `json:"opted_out_later"`
}

type parsedImport struct {
	Rows      []importRow  `json:"rows"`
	Invalid   []InvalidRow `json:"invalid"`
	RowsTotal int          `json:"rows_total"`
}

// importPhone turns a number as typed into digits with the country code:
// Australian local numbers (0…) get 61 (Weline's first market).
func importPhone(s string) (string, bool) {
	d := nonDigits.ReplaceAllString(s, "")
	d = strings.TrimPrefix(d, "00") // international prefix
	if len(d) == 10 && strings.HasPrefix(d, "0") {
		d = "61" + d[1:]
	}
	return d, len(d) >= 8 && len(d) <= 15
}

func (s *Service) parseCSV(data []byte) (*parsedImport, error) {
	if len(data) == 0 {
		return nil, kit.Invalid("file: the file is empty")
	}
	if len(data) > maxImportBytes {
		return nil, kit.Invalid("file: larger than 1 MB")
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return nil, kit.Invalid("file: not a CSV file")
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	phoneCol, ok1 := col["phone"]
	dateCol, ok2 := col["given_at"]
	noteCol, hasNote := col["note"]
	if !ok1 || !ok2 {
		return nil, kit.Invalid("file: needs the columns phone and given_at (and optionally note)")
	}
	out := &parsedImport{Rows: []importRow{}, Invalid: []InvalidRow{}}
	seen := map[string]int{} // phone → index in Rows
	line := 1
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err != nil {
			return nil, kit.Invalid("file: line %d can't be read", line)
		}
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue
		}
		out.RowsTotal++
		if out.RowsTotal > maxImportRows {
			return nil, kit.Invalid("file: more than %d rows", maxImportRows)
		}
		cell := func(i int) string {
			if i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		phone, ok := importPhone(cell(phoneCol))
		if !ok {
			out.Invalid = append(out.Invalid, InvalidRow{Row: line, Reason: "phone number not valid"})
			continue
		}
		given, err := s.parseGivenAt(cell(dateCol))
		if err != nil || cell(dateCol) == "" {
			out.Invalid = append(out.Invalid, InvalidRow{Row: line, Reason: "given_at must be a date like 2026-03-01, not in the future"})
			continue
		}
		note := ""
		if hasNote {
			note = cell(noteCol)
		}
		if len([]rune(note)) > maxEvidence {
			out.Invalid = append(out.Invalid, InvalidRow{Row: line, Reason: fmt.Sprintf("note longer than %d characters", maxEvidence)})
			continue
		}
		if i, dup := seen[phone]; dup {
			out.Invalid = append(out.Invalid, InvalidRow{Row: line, Reason: "number already in the file"})
			if given.After(out.Rows[i].GivenAt) {
				out.Rows[i] = importRow{Line: line, Phone: phone, GivenAt: given, Note: note}
			}
			continue
		}
		seen[phone] = len(out.Rows)
		out.Rows = append(out.Rows, importRow{Line: line, Phone: phone, GivenAt: given, Note: note})
	}
	if out.RowsTotal == 0 {
		return nil, kit.Invalid("file: no rows under the header")
	}
	return out, nil
}

type importMatch struct {
	row       importRow
	contactID uuid.UUID
}

type importPlan struct {
	apply          []importMatch
	alreadyOptedIn int
	unknown        int
	optedOutLater  int
}

// plan matches rows to the organization's contacts and sorts them.
func (s *Service) plan(ctx context.Context, orgID uuid.UUID, p *parsedImport) (*importPlan, error) {
	out := &importPlan{}
	if len(p.Rows) == 0 {
		return out, nil
	}
	phones := make([]string, 0, len(p.Rows))
	for _, r := range p.Rows {
		phones = append(phones, r.Phone)
	}
	var found []struct {
		ID              uuid.UUID
		Digits          string
		MarketingOptOut bool
		Marketing       *string
		GivenAt         *time.Time
	}
	err := s.db.WithContext(ctx).Raw(`SELECT DISTINCT ON (d.digits) c.id, d.digits, c.marketing_opt_out, cs.marketing, cs.given_at
		FROM contacts c
		CROSS JOIN LATERAL (SELECT regexp_replace(c.phone_number, '[^0-9]', '', 'g') AS digits) d
		LEFT JOIN wl.consent_status cs ON cs.organization_id = c.organization_id AND cs.contact_id = c.id
		WHERE c.organization_id = ? AND c.deleted_at IS NULL AND d.digits IN ?
		ORDER BY d.digits, c.created_at`, orgID, phones).Scan(&found).Error
	if err != nil {
		return nil, fmt.Errorf("match contacts: %w", err)
	}
	byPhone := map[string]int{}
	for i, f := range found {
		byPhone[f.Digits] = i
	}
	for _, r := range p.Rows {
		i, ok := byPhone[r.Phone]
		if !ok {
			out.unknown++
			continue
		}
		f := found[i]
		switch {
		case f.MarketingOptOut || (f.Marketing != nil && *f.Marketing == OptedOut):
			// A flag Meta set without a recorded time counts as just now.
			outAt := s.now()
			if f.Marketing != nil && *f.Marketing == OptedOut && f.GivenAt != nil {
				outAt = *f.GivenAt
			}
			if outAt.After(r.GivenAt) {
				out.optedOutLater++
				continue
			}
		case f.Marketing != nil && *f.Marketing == OptedIn:
			out.alreadyOptedIn++
			continue
		}
		out.apply = append(out.apply, importMatch{row: r, contactID: f.ID})
	}
	return out, nil
}

func (s *Service) importKey(orgID uuid.UUID, token string) string {
	return "wl:consent:import:" + orgID.String() + ":" + token
}

// PreviewImport parses a CSV of opt-ins and says what applying would do. The
// parsed rows are kept for 15 minutes under a token.
func (s *Service) PreviewImport(ctx context.Context, orgID uuid.UUID, data []byte) (*ImportPreview, error) {
	if s.rdb == nil {
		return nil, kit.NotReady("imports need Redis")
	}
	parsed, err := s.parseCSV(data)
	if err != nil {
		return nil, err
	}
	pl, err := s.plan(ctx, orgID, parsed)
	if err != nil {
		return nil, err
	}
	token := uuid.NewString()
	b, err := json.Marshal(parsed)
	if err != nil {
		return nil, fmt.Errorf("encode import: %w", err)
	}
	if err := s.rdb.Set(ctx, s.importKey(orgID, token), b, importTTL).Err(); err != nil {
		return nil, fmt.Errorf("keep import: %w", err)
	}
	listed := parsed.Invalid
	if len(listed) > maxInvalidListed {
		listed = listed[:maxInvalidListed]
	}
	return &ImportPreview{Token: token, RowsTotal: parsed.RowsTotal, WillApply: len(pl.apply), AlreadyOptedIn: pl.alreadyOptedIn,
		UnknownNumbers: pl.unknown, Invalid: len(parsed.Invalid), InvalidRows: listed, OptedOutLater: pl.optedOutLater}, nil
}

// ImportInput applies a previewed file.
type ImportInput struct {
	Token        string `json:"token"`
	Filename     string `json:"filename"`
	Evidence     string `json:"evidence"`
	ConfirmCount int    `json:"confirm_count"`
}

// ImportResult is what was applied.
type ImportResult struct {
	ID                   uuid.UUID `json:"id"`
	Applied              int       `json:"applied"`
	SkippedUnknown       int       `json:"skipped_unknown"`
	SkippedInvalid       int       `json:"skipped_invalid"`
	SkippedOptedOutLater int       `json:"skipped_opted_out_later"`
	AlreadyOptedIn       int       `json:"already_opted_in"`
}

type importRecord struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID       uuid.UUID `gorm:"type:uuid"`
	Filename             string
	Evidence             string
	RowsTotal            int
	Applied              int
	SkippedUnknown       int
	SkippedInvalid       int
	SkippedOptedOutLater int
	CreatedByID          *uuid.UUID `gorm:"type:uuid"`
	CreatedAt            time.Time
}

func (importRecord) TableName() string { return "wl.consent_imports" }

// ApplyImport records the opt-ins of a previewed file.
func (s *Service) ApplyImport(ctx context.Context, orgID, userID uuid.UUID, in ImportInput) (*ImportResult, error) {
	if s.rdb == nil {
		return nil, kit.NotReady("imports need Redis")
	}
	evidence := strings.TrimSpace(in.Evidence)
	if evidence == "" {
		return nil, kit.Invalid("evidence: describe the proof of consent for this file")
	}
	if len([]rune(evidence)) > maxImportEvidence {
		return nil, kit.Invalid("evidence: at most %d characters", maxImportEvidence)
	}
	if _, err := uuid.Parse(in.Token); err != nil {
		return nil, kit.Invalid("token: preview the file first")
	}
	key := s.importKey(orgID, in.Token)
	b, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, kit.Gone("the preview expired; upload the file again")
	}
	if err != nil {
		return nil, fmt.Errorf("load import: %w", err)
	}
	var parsed parsedImport
	if err := json.Unmarshal(b, &parsed); err != nil {
		return nil, fmt.Errorf("decode import: %w", err)
	}
	pl, err := s.plan(ctx, orgID, &parsed)
	if err != nil {
		return nil, err
	}
	if len(pl.apply) != in.ConfirmCount {
		return nil, kit.Conflict("contacts changed since the preview (now %d to apply, you confirmed %d); preview again", len(pl.apply), in.ConfirmCount)
	}
	filename := strings.TrimSpace(in.Filename)
	if len([]rune(filename)) > 255 {
		filename = string([]rune(filename)[:255])
	}
	rec := importRecord{OrganizationID: orgID, Filename: filename, Evidence: evidence, RowsTotal: parsed.RowsTotal,
		SkippedUnknown: pl.unknown, SkippedInvalid: len(parsed.Invalid), SkippedOptedOutLater: pl.optedOutLater,
		CreatedByID: &userID, CreatedAt: s.now()}
	if err := s.db.WithContext(ctx).Create(&rec).Error; err != nil {
		return nil, fmt.Errorf("save import: %w", err)
	}
	for _, m := range pl.apply {
		res, err := s.Apply(ctx, orgID, m.contactID, Change{Action: ActionOptIn, Source: SourceImport, GivenAt: m.row.GivenAt,
			Evidence: m.row.Note, RecordedBy: &userID, ImportID: &rec.ID})
		switch {
		case err != nil:
			return nil, err
		case res.Skipped != "":
			rec.SkippedOptedOutLater++
		case res.Recorded:
			rec.Applied++
		}
	}
	if err := s.db.WithContext(ctx).Model(&rec).Where("organization_id = ?", orgID).
		Updates(map[string]any{"applied": rec.Applied, "skipped_opted_out_later": rec.SkippedOptedOutLater}).Error; err != nil {
		return nil, fmt.Errorf("save import counts: %w", err)
	}
	_ = s.rdb.Del(ctx, key).Err()
	return &ImportResult{ID: rec.ID, Applied: rec.Applied, SkippedUnknown: rec.SkippedUnknown, SkippedInvalid: rec.SkippedInvalid,
		SkippedOptedOutLater: rec.SkippedOptedOutLater, AlreadyOptedIn: pl.alreadyOptedIn}, nil
}
