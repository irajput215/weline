package audiences

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// Rules is a segment's definition (design.md, "Rules"). It is JSON with a
// closed set of fields and operators, compiled to parameterised SQL: no user
// text ever becomes SQL.
type Rules struct {
	Match      string      `json:"match"` // all | any
	Conditions []Condition `json:"conditions"`
}

// Condition is one rule row.
type Condition struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Key   string          `json:"key,omitempty"`   // metadata key
	Value json.RawMessage `json:"value,omitempty"` // string, number or campaign id, by field
	Days  int             `json:"days,omitempty"`  // campaign reply windows
}

const (
	maxConditions   = 20
	maxDays         = 365
	defaultReplyDay = 7
)

var metadataKeyRe = regexp.MustCompile(`^[a-zA-Z0-9_]{1,50}$`)

var fieldOps = map[string][]string{
	"tag":              {"has", "has_not"},
	"metadata":         {"eq", "neq", "contains", "exists", "not_exists"},
	"whatsapp_account": {"eq"},
	"last_inbound":     {"within_days", "not_within_days"},
	"created":          {"within_days", "not_within_days"},
	"campaign":         {"received", "not_received", "read", "not_read", "read_no_reply", "replied"},
	"consent":          {"opted_in", "opted_out", "not_recorded"}, // Consent module (2.2 R9)
}

// RetargetFilters are the campaign ops a retarget can use (R8).
var RetargetFilters = []string{"not_received", "not_read", "read_no_reply", "replied"}

func allowed(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (c Condition) str() (string, error) {
	var s string
	if err := json.Unmarshal(c.Value, &s); err != nil {
		return "", kit.Invalid("%s: value must be text", c.Field)
	}
	return strings.TrimSpace(s), nil
}

func (c Condition) days() (int, error) {
	var n int
	if err := json.Unmarshal(c.Value, &n); err != nil || n < 1 || n > maxDays {
		return 0, kit.Invalid("%s: days must be between 1 and %d", c.Field, maxDays)
	}
	return n, nil
}

// Validate checks the rules without touching the database.
func (r *Rules) Validate() error {
	if r.Match == "" {
		r.Match = "all"
	}
	if r.Match != "all" && r.Match != "any" {
		return kit.Invalid("match must be all or any")
	}
	if len(r.Conditions) == 0 {
		return kit.Invalid("add at least one condition")
	}
	if len(r.Conditions) > maxConditions {
		return kit.Invalid("at most %d conditions", maxConditions)
	}
	for i, c := range r.Conditions {
		ops, ok := fieldOps[c.Field]
		if !ok {
			return kit.Invalid("condition %d: unknown field %q", i+1, c.Field)
		}
		if !allowed(ops, c.Op) {
			return kit.Invalid("condition %d: %s can't use %q", i+1, c.Field, c.Op)
		}
		switch c.Field {
		case "metadata":
			if !metadataKeyRe.MatchString(c.Key) {
				return kit.Invalid("condition %d: contact field names use letters, digits and _ (max 50)", i+1)
			}
			if c.Op != "exists" && c.Op != "not_exists" {
				if _, err := c.str(); err != nil {
					return err
				}
			}
		case "tag", "whatsapp_account":
			v, err := c.str()
			if err != nil {
				return err
			}
			if v == "" {
				return kit.Invalid("condition %d: choose a %s", i+1, strings.ReplaceAll(c.Field, "_", " "))
			}
		case "last_inbound", "created":
			if _, err := c.days(); err != nil {
				return err
			}
		case "consent":
			// no value
		case "campaign":
			v, err := c.str()
			if err != nil {
				return err
			}
			if _, err := uuid.Parse(v); err != nil {
				return kit.Invalid("condition %d: choose a campaign", i+1)
			}
			if c.Days < 0 || c.Days > maxDays {
				return kit.Invalid("condition %d: days must be between 1 and %d", i+1, maxDays)
			}
		}
	}
	return nil
}

// campaignIDs returns the campaigns the rules refer to, to check they belong
// to the organization.
func (r *Rules) campaignIDs() []uuid.UUID {
	var out []uuid.UUID
	for _, c := range r.Conditions {
		if c.Field == "campaign" {
			if v, err := c.str(); err == nil {
				if id, err := uuid.Parse(v); err == nil {
					out = append(out, id)
				}
			}
		}
	}
	return out
}

// digits normalises a phone column in SQL the same way normalisePhone does in Go.
const digitsSQL = "regexp_replace(%s, '[^0-9]', '', 'g')"

// compile returns a WHERE clause over contacts aliased "c" (organization filter
// included) and its arguments. Validate must have passed.
func (r *Rules) compile(orgID uuid.UUID) (string, []any) {
	parts := make([]string, 0, len(r.Conditions))
	var args []any
	recipientPhone := fmt.Sprintf(digitsSQL, "r.phone_number")
	contactPhone := fmt.Sprintf(digitsSQL, "c.phone_number")
	for _, c := range r.Conditions {
		switch c.Field {
		case "tag":
			v, _ := c.str()
			tag, _ := json.Marshal([]string{v})
			sql := "COALESCE(c.tags, '[]'::jsonb) @> ?::jsonb"
			if c.Op == "has_not" {
				sql = "NOT (" + sql + ")"
			}
			parts = append(parts, sql)
			args = append(args, string(tag))
		case "metadata":
			switch c.Op {
			case "exists":
				parts = append(parts, "COALESCE(c.metadata->>?, '') <> ''")
				args = append(args, c.Key)
			case "not_exists":
				parts = append(parts, "COALESCE(c.metadata->>?, '') = ''")
				args = append(args, c.Key)
			case "eq":
				v, _ := c.str()
				parts = append(parts, "lower(c.metadata->>?) = lower(?)")
				args = append(args, c.Key, v)
			case "neq":
				v, _ := c.str()
				parts = append(parts, "lower(COALESCE(c.metadata->>?, '')) <> lower(?)")
				args = append(args, c.Key, v)
			case "contains":
				v, _ := c.str()
				parts = append(parts, "c.metadata->>? ILIKE ?")
				args = append(args, c.Key, "%"+likeEscape(v)+"%")
			}
		case "consent":
			status := "EXISTS (SELECT 1 FROM wl.consent_status cs WHERE cs.organization_id = c.organization_id AND cs.contact_id = c.id%s)"
			switch c.Op {
			case "opted_in":
				parts = append(parts, "(NOT c.marketing_opt_out AND "+fmt.Sprintf(status, " AND cs.marketing = 'opted_in'")+")")
			case "opted_out":
				parts = append(parts, "(c.marketing_opt_out OR "+fmt.Sprintf(status, " AND cs.marketing = 'opted_out'")+")")
			case "not_recorded":
				parts = append(parts, "(NOT c.marketing_opt_out AND NOT "+fmt.Sprintf(status, "")+")")
			}
		case "whatsapp_account":
			v, _ := c.str()
			parts = append(parts, "c.whats_app_account = ?")
			args = append(args, v)
		case "last_inbound", "created":
			col := "c.last_inbound_at"
			if c.Field == "created" {
				col = "c.created_at"
			}
			n, _ := c.days()
			if c.Op == "within_days" {
				parts = append(parts, col+" >= now() - make_interval(days => ?)")
			} else {
				parts = append(parts, "("+col+" IS NULL OR "+col+" < now() - make_interval(days => ?))")
			}
			args = append(args, n)
		case "campaign":
			v, _ := c.str()
			days := c.Days
			if days == 0 {
				days = defaultReplyDay
			}
			base := "EXISTS (SELECT 1 FROM bulk_message_recipients r JOIN bulk_message_campaigns bc ON bc.id = r.campaign_id " +
				"WHERE bc.organization_id = ? AND r.campaign_id = ? AND r.deleted_at IS NULL AND " + recipientPhone + " = " + contactPhone + " AND %s)"
			replied := "EXISTS (SELECT 1 FROM messages m WHERE m.organization_id = ? AND m.contact_id = c.id AND m.direction = 'incoming' " +
				"AND m.created_at > %s AND m.created_at <= %s + make_interval(days => ?))"
			var cond string
			var extra []any
			switch c.Op {
			case "received":
				cond = "r.status IN ('delivered', 'read')"
			case "not_received":
				cond = "(r.status = 'failed' OR (r.status = 'sent' AND r.sent_at < now() - interval '24 hours'))"
			case "read":
				cond = "r.status = 'read'"
			case "not_read":
				cond = "r.status = 'delivered'"
			case "read_no_reply":
				cond = "r.status = 'read' AND NOT " + fmt.Sprintf(replied, "r.read_at", "r.read_at")
				extra = []any{orgID, days}
			case "replied":
				cond = "r.sent_at IS NOT NULL AND " + fmt.Sprintf(replied, "r.sent_at", "r.sent_at")
				extra = []any{orgID, days}
			}
			parts = append(parts, fmt.Sprintf(base, cond))
			args = append(args, orgID, v)
			args = append(args, extra...)
		}
	}
	joiner := " AND "
	if r.Match == "any" {
		joiner = " OR "
	}
	where := "c.organization_id = ? AND c.deleted_at IS NULL AND (" + strings.Join(parts, joiner) + ")"
	return where, append([]any{orgID}, args...)
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}
