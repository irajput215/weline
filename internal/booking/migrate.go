package booking

import (
	"github.com/zerodha/logf"
	"gorm.io/gorm"
)

// Migrate creates or updates booking_* tables.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Appointment{},
	)
}

// MustMigrate runs Migrate and exits on failure. It is called from a one-line
// hook in cmd/whatomate/main.go right after the upstream migrations.
func MustMigrate(db *gorm.DB, lo logf.Logger) {
	if err := Migrate(db); err != nil {
		lo.Fatal("Booking migration failed", "error", err)
	}
}
