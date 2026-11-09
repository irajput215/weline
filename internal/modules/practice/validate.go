package practice

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// TimeZones are the clinic time zones offered (R2), default first.
var TimeZones = []string{
	"Australia/Sydney", "Australia/Melbourne", "Australia/Brisbane", "Australia/Adelaide",
	"Australia/Darwin", "Australia/Perth", "Australia/Hobart",
}

// DefaultTimeZone is used when a clinic doesn't choose one.
const DefaultTimeZone = "Australia/Sydney"

// SlotLengths are the allowed slot lengths in minutes (R4).
var SlotLengths = []int{5, 10, 15, 20, 30, 60}

// DefaultSlotMinutes is the default slot length.
const DefaultSlotMinutes = 15

// Field length limits, matching the column sizes.
const (
	maxName    = 120
	maxAddress = 255
	maxPhone   = 40
	maxEmail   = 254
	maxWebsite = 255
	maxRole    = 60
	maxReason  = 120
)

var (
	slugRe  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{1,38}[a-z0-9])$`)
	ahpraRe = regexp.MustCompile(`^[A-Z]{3}[0-9]{10}$`)
	abnRe   = regexp.MustCompile(`^[0-9]{11}$`)
)

// reservedSlugs can't be booking slugs: they collide with app paths or could
// be used to impersonate Weline.
var reservedSlugs = map[string]bool{
	"admin": true, "api": true, "app": true, "assets": true, "book": true, "booking": true,
	"help": true, "login": true, "logout": true, "register": true, "settings": true,
	"static": true, "support": true, "weline": true, "whatomate": true, "www": true, "x": true,
}

func validTimeZone(tz string) bool {
	for _, z := range TimeZones {
		if z == tz {
			return true
		}
	}
	return false
}

func validSlot(m int) bool {
	for _, s := range SlotLengths {
		if s == m {
			return true
		}
	}
	return false
}

// validateSlug checks R3's format and reserved words. Uniqueness is checked
// against the database by the caller.
func validateSlug(slug string) error {
	if !slugRe.MatchString(slug) || strings.Contains(slug, "--") {
		return kit.Invalid("booking slug must be 3-40 lowercase letters, digits or single hyphens, starting and ending with a letter or digit")
	}
	if reservedSlugs[slug] {
		return kit.Invalid("booking slug %q is reserved", slug)
	}
	return nil
}

// validABN checks an 11-digit ABN with the ATO checksum: subtract 1 from the
// first digit, weight the digits, and the sum must divide by 89 (R5).
func validABN(abn string) bool {
	if !abnRe.MatchString(abn) {
		return false
	}
	weights := []int{10, 1, 3, 5, 7, 9, 11, 13, 15, 17, 19}
	sum := 0
	for i, r := range abn {
		d := int(r - '0')
		if i == 0 {
			d--
		}
		sum += d * weights[i]
	}
	return sum%89 == 0
}

// validAHPRA checks the AHPRA registration number format: 3 letters + 10 digits (R8).
func validAHPRA(n string) bool { return ahpraRe.MatchString(n) }

// required trims s and checks it is 1..max characters.
func required(field, s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", kit.Invalid("%s is required", field)
	}
	if utf8.RuneCountInString(s) > max {
		return "", kit.Invalid("%s must be at most %d characters", field, max)
	}
	return s, nil
}

// optional trims s and checks it is at most max characters.
func optional(field, s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		return "", kit.Invalid("%s must be at most %d characters", field, max)
	}
	return s, nil
}

func validEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s
}

// formatMinute renders minutes after midnight as "HH:MM" (1440 -> "24:00").
func formatMinute(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }

// parseMinute parses "HH:MM" (00:00-24:00) into minutes after midnight.
func parseMinute(s string) (int, error) {
	var h, m int
	if len(s) != 5 || s[2] != ':' {
		return 0, kit.Invalid("time %q must be HH:MM", s)
	}
	if _, err := fmt.Sscanf(s, "%02d:%02d", &h, &m); err != nil || h > 24 || m > 59 || (h == 24 && m != 0) {
		return 0, kit.Invalid("time %q must be HH:MM between 00:00 and 24:00", s)
	}
	return h*60 + m, nil
}
