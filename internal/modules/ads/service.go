package ads

import (
	"context"

	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

// Logger is the part of Weline's logger the module uses.
type Logger interface {
	Info(msg string, fields ...any)
	Error(msg string, fields ...any)
}

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

// Service holds the ads rules. Every method takes the organization first and
// filters every query by it.
type Service struct {
	db  *gorm.DB
	log Logger
}

// NewService returns the ads service.
func NewService(db *gorm.DB, log Logger) *Service {
	if log == nil {
		log = nopLogger{}
	}
	return &Service{db: db, log: log}
}

// Events implements kit.EventHandler.
func (m *Module) Events() []string { return []string{"message.incoming"} }

// HandleEvent implements kit.EventHandler.
func (m *Module) HandleEvent(ctx context.Context, e kit.Event) error { return nil }
