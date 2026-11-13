package ai

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

const (
	chunkSize     = 800
	maxDocument   = 200 * 1024
	maxPassages   = 4
	maxQueryTerms = 20
)

type knowledgeRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrganizationID uuid.UUID `gorm:"type:uuid"`
	SourceID       uuid.UUID `gorm:"type:uuid"`
	Kind           string
	Title          string
	Chunk          int
	Content        string
	CreatedByID    *uuid.UUID `gorm:"type:uuid;column:created_by_id"`
	CreatedAt      time.Time
}

func (knowledgeRow) TableName() string { return "wl.ai_knowledge" }

// KnowledgeInput adds an FAQ entry or a document (R3).
type KnowledgeInput struct {
	Kind     string `json:"kind"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Title    string `json:"title"`
	Text     string `json:"text"`
}

// KnowledgeItem is one FAQ or document as the list shows it.
type KnowledgeItem struct {
	SourceID  uuid.UUID `json:"source_id"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Chunks    int       `json:"chunks"`
	CreatedAt time.Time `json:"created_at"`
}

var paraRe = regexp.MustCompile(`\n\s*\n`)

// chunk splits text into pieces of about chunkSize characters, on paragraph
// and then sentence boundaries.
func chunk(text string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	for _, para := range paraRe.Split(strings.ReplaceAll(text, "\r\n", "\n"), -1) {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if cur.Len()+len(para) > chunkSize && cur.Len() > 0 {
			flush()
		}
		for len(para) > chunkSize {
			cut := strings.LastIndexAny(para[:chunkSize], ".!?\n")
			if cut < chunkSize/2 {
				cut = strings.LastIndex(para[:chunkSize], " ")
			}
			if cut <= 0 {
				cut = chunkSize
			}
			cur.WriteString(para[:cut+1])
			flush()
			para = strings.TrimSpace(para[cut+1:])
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(para)
	}
	flush()
	return out
}

// AddKnowledge adds an FAQ entry or a document.
func (s *Service) AddKnowledge(ctx context.Context, orgID, userID uuid.UUID, in KnowledgeInput) (*KnowledgeItem, error) {
	var title string
	var pieces []string
	switch in.Kind {
	case "faq":
		q, a := strings.TrimSpace(in.Question), strings.TrimSpace(in.Answer)
		if q == "" || a == "" {
			return nil, kit.Invalid("question and answer are required")
		}
		if len(q) > 200 || len(a) > 4000 {
			return nil, kit.Invalid("question: at most 200 characters; answer: at most 4,000")
		}
		title, pieces = q, []string{a}
	case "document":
		title = strings.TrimSpace(in.Title)
		if title == "" || len(title) > 200 {
			return nil, kit.Invalid("title: 1 to 200 characters")
		}
		if len(in.Text) > maxDocument {
			return nil, kit.Invalid("text: at most 200 KB")
		}
		pieces = chunk(in.Text)
		if len(pieces) == 0 {
			return nil, kit.Invalid("text is empty")
		}
	default:
		return nil, kit.Invalid("kind: faq or document")
	}
	src := uuid.New()
	now := s.now()
	rows := make([]knowledgeRow, 0, len(pieces))
	for i, p := range pieces {
		rows = append(rows, knowledgeRow{OrganizationID: orgID, SourceID: src, Kind: in.Kind, Title: title, Chunk: i, Content: p,
			CreatedByID: &userID, CreatedAt: now})
	}
	if err := s.db.WithContext(ctx).Create(&rows).Error; err != nil {
		return nil, fmt.Errorf("save knowledge: %w", err)
	}
	return &KnowledgeItem{SourceID: src, Kind: in.Kind, Title: title, Chunks: len(rows), CreatedAt: now}, nil
}

// ListKnowledge lists FAQ entries and documents, newest first.
func (s *Service) ListKnowledge(ctx context.Context, orgID uuid.UUID) ([]KnowledgeItem, error) {
	out := []KnowledgeItem{}
	err := s.db.WithContext(ctx).Raw(`SELECT source_id, min(kind) AS kind, min(title) AS title, count(*) AS chunks, min(created_at) AS created_at
		FROM wl.ai_knowledge WHERE organization_id = ? GROUP BY source_id ORDER BY min(created_at) DESC`, orgID).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("list knowledge: %w", err)
	}
	return out, nil
}

// DeleteKnowledge removes an FAQ entry or document.
func (s *Service) DeleteKnowledge(ctx context.Context, orgID, sourceID uuid.UUID) error {
	res := s.db.WithContext(ctx).Where("organization_id = ? AND source_id = ?", orgID, sourceID).Delete(&knowledgeRow{})
	if res.Error != nil {
		return fmt.Errorf("delete knowledge: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return kit.NotFound("not found")
	}
	return nil
}

type passage struct {
	SourceID uuid.UUID
	Title    string
	Content  string
	Rank     float64
}

// searchTerms keeps letters and digits of words of 3+ characters, joined with
// OR, so a natural question matches passages sharing any of its words.
func searchTerms(q string) string {
	words := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var keep []string
	seen := map[string]bool{}
	for _, w := range words {
		if len([]rune(w)) >= 3 && !seen[w] && len(keep) < maxQueryTerms {
			seen[w] = true
			keep = append(keep, w)
		}
	}
	return strings.Join(keep, " | ")
}

// Search finds the passages most relevant to a question (R3).
func (s *Service) Search(ctx context.Context, orgID uuid.UUID, q string) ([]passage, error) {
	terms := searchTerms(q)
	if terms == "" {
		return nil, nil
	}
	var out []passage
	err := s.db.WithContext(ctx).Raw(`SELECT source_id, title, content, ts_rank(search, query) AS rank
		FROM wl.ai_knowledge, to_tsquery('english', ?) query
		WHERE organization_id = ? AND search @@ query ORDER BY rank DESC, id LIMIT ?`, terms, orgID, maxPassages).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("search knowledge: %w", err)
	}
	return out, nil
}
