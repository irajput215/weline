package example

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// MaxNoteLength is the longest note accepted, in characters.
const MaxNoteLength = 1000

// listLimit caps how many notes List returns.
const listLimit = 100

// Note is one row of wl.example_notes.
type Note struct {
	models.BaseModel
	OrganizationID uuid.UUID  `gorm:"type:uuid;not null" json:"organization_id"`
	Text           string     `gorm:"type:text;not null" json:"text"`
	CreatedByID    *uuid.UUID `gorm:"type:uuid" json:"created_by_id,omitempty"`
}

// TableName puts notes in the module's own schema.
func (Note) TableName() string { return "wl.example_notes" }

// Service holds the notes rules. Every method takes orgID first and only
// ever touches that organization's rows.
type Service struct {
	db *gorm.DB
}

// NewService returns a Service backed by db.
func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// List returns the organization's newest notes first.
func (s *Service) List(ctx context.Context, orgID uuid.UUID) ([]Note, error) {
	notes := []Note{}
	err := s.db.WithContext(ctx).
		Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Limit(listLimit).
		Find(&notes).Error
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	return notes, nil
}

// Create adds a note for the organization. The text is trimmed and must be
// 1 to MaxNoteLength characters.
func (s *Service) Create(ctx context.Context, orgID, userID uuid.UUID, text string) (*Note, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, kit.Invalid("note text is required")
	}
	if utf8.RuneCountInString(text) > MaxNoteLength {
		return nil, kit.Invalid("note text must be at most %d characters", MaxNoteLength)
	}

	note := &Note{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		Text:           text,
		CreatedByID:    &userID,
	}
	if err := s.db.WithContext(ctx).Create(note).Error; err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}
	return note, nil
}
