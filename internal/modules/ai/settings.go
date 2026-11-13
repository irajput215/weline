package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm/clause"
)

// Settings are the organization's agent settings (R2).
type Settings struct {
	Enabled             bool       `json:"enabled"`
	HandoverTeamID      *uuid.UUID `json:"handover_team_id"`
	Tone                string     `json:"tone"`
	Instructions        string     `json:"instructions"`
	ConfidenceThreshold float64    `json:"confidence_threshold"`
	EmergencyWords      []string   `json:"emergency_words"`
}

type settingsRow struct {
	OrganizationID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	Enabled             bool
	HandoverTeamID      *uuid.UUID `gorm:"type:uuid"`
	Tone                string
	Instructions        string
	ConfidenceThreshold float64
	EmergencyWords      json.RawMessage `gorm:"type:jsonb"`
	UpdatedByID         *uuid.UUID      `gorm:"type:uuid"`
	UpdatedAt           time.Time
}

func (settingsRow) TableName() string { return "wl.ai_settings" }

// DefaultSettings: off, friendly, 0.7, the default emergency words.
func DefaultSettings() Settings {
	return Settings{Tone: "friendly", ConfidenceThreshold: 0.7, EmergencyWords: append([]string{}, DefaultEmergencyWords...)}
}

// GetSettings returns the organization's settings, or the defaults.
func (s *Service) GetSettings(ctx context.Context, orgID uuid.UUID) (Settings, error) {
	var rows []settingsRow
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Limit(1).Find(&rows).Error; err != nil {
		return Settings{}, fmt.Errorf("load AI settings: %w", err)
	}
	if len(rows) == 0 {
		return DefaultSettings(), nil
	}
	r := rows[0]
	st := Settings{Enabled: r.Enabled, HandoverTeamID: r.HandoverTeamID, Tone: r.Tone, Instructions: r.Instructions,
		ConfidenceThreshold: r.ConfidenceThreshold}
	_ = json.Unmarshal(r.EmergencyWords, &st.EmergencyWords)
	if len(st.EmergencyWords) == 0 {
		st.EmergencyWords = append([]string{}, DefaultEmergencyWords...)
	}
	return st, nil
}

var tones = map[string]bool{"friendly": true, "formal": true, "brief": true}

// SaveSettings checks and saves the settings.
func (s *Service) SaveSettings(ctx context.Context, orgID, userID uuid.UUID, in Settings) (Settings, error) {
	if in.Tone == "" {
		in.Tone = "friendly"
	}
	if !tones[in.Tone] {
		return Settings{}, kit.Invalid("tone: friendly, formal or brief")
	}
	in.Instructions = strings.TrimSpace(in.Instructions)
	if len([]rune(in.Instructions)) > 4000 {
		return Settings{}, kit.Invalid("instructions: at most 4,000 characters")
	}
	if in.ConfidenceThreshold == 0 {
		in.ConfidenceThreshold = 0.7
	}
	if in.ConfidenceThreshold < 0.3 || in.ConfidenceThreshold > 0.95 {
		return Settings{}, kit.Invalid("confidence_threshold: between 0.3 and 0.95")
	}
	words := []string{}
	seen := map[string]bool{}
	for _, w := range in.EmergencyWords {
		w = strings.ToLower(strings.TrimSpace(w))
		if w != "" && !seen[w] && len(w) <= 50 {
			seen[w] = true
			words = append(words, w)
		}
	}
	if len(words) == 0 {
		return Settings{}, kit.Invalid("emergency_words: keep at least one")
	}
	if len(words) > 100 {
		return Settings{}, kit.Invalid("emergency_words: at most 100")
	}
	in.EmergencyWords = words
	b, _ := json.Marshal(words)
	row := settingsRow{OrganizationID: orgID, Enabled: in.Enabled, HandoverTeamID: in.HandoverTeamID, Tone: in.Tone,
		Instructions: in.Instructions, ConfidenceThreshold: in.ConfidenceThreshold, EmergencyWords: b, UpdatedByID: &userID, UpdatedAt: s.now()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error; err != nil {
		return Settings{}, fmt.Errorf("save AI settings: %w", err)
	}
	return in, nil
}
