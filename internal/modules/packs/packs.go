package packs

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

//go:embed data/*.json
var dataFS embed.FS

// Pack is one industry's starting set.
type Pack struct {
	ID          string     `json:"id"`
	Version     int        `json:"version"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Verticals   []string   `json:"verticals"`  // onboarding verticals it fits
	Connectors  []string   `json:"connectors"` // connector providers that fit
	Templates   []Template `json:"templates"`
	AI          AI         `json:"ai"`
	Segments    []Segment  `json:"segments"`
	Reminders   []Reminder `json:"reminders"`
	Flows       []Flow     `json:"flows"`
}

// Template is a WhatsApp template to submit to Meta.
type Template struct {
	Name     string   `json:"name"`
	Language string   `json:"language"`
	Category string   `json:"category"` // UTILITY or MARKETING
	Body     string   `json:"body"`
	Footer   string   `json:"footer,omitempty"`
	Buttons  []string `json:"buttons,omitempty"` // quick replies
	Samples  []string `json:"samples"`           // one per {{n}}, for Meta's review
}

// AI is the agent's setup for the industry.
type AI struct {
	Tone           string   `json:"tone"`
	Instructions   string   `json:"instructions"`
	EmergencyWords []string `json:"emergency_words"`
}

// Segment is a suggested audience (Audiences rules).
type Segment struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Rules       json.RawMessage `json:"rules"`
}

// Reminder is a suggested reminder or recall rule.
type Reminder struct {
	Kind          string   `json:"kind"`
	Name          string   `json:"name"`
	OffsetMinutes int      `json:"offset_minutes,omitempty"`
	RecallDays    int      `json:"recall_days,omitempty"`
	Template      string   `json:"template"` // one of the pack's templates
	Variables     []string `json:"variables"`
}

// Flow points to a chatbot flow to build in Weline's flow builder.
type Flow struct {
	Name  string `json:"name"`
	Guide string `json:"guide"`
}

var (
	idRe       = regexp.MustCompile(`^[a-z][a-z0-9_]{1,30}$`)
	templateRe = regexp.MustCompile(`^[a-z0-9_]{1,512}$`) // Meta's rule
	varRe      = regexp.MustCompile(`\{\{(\d+)\}\}`)
)

// validate checks a pack is safe to install.
func (p Pack) validate() error {
	if !idRe.MatchString(p.ID) || p.Version < 1 || p.Name == "" {
		return fmt.Errorf("pack %q: id, version and name are required", p.ID)
	}
	names := map[string]bool{}
	for _, t := range p.Templates {
		if !templateRe.MatchString(t.Name) || !strings.HasPrefix(t.Name, "wl_"+p.ID+"_") {
			return fmt.Errorf("pack %s: template %q must be lowercase and start with wl_%s_", p.ID, t.Name, p.ID)
		}
		if t.Category != "UTILITY" && t.Category != "MARKETING" {
			return fmt.Errorf("pack %s: template %s: category must be UTILITY or MARKETING", p.ID, t.Name)
		}
		if t.Category == "MARKETING" && !strings.Contains(strings.ToUpper(t.Footer), "STOP") {
			return fmt.Errorf("pack %s: marketing template %s must say how to stop (Reply STOP)", p.ID, t.Name)
		}
		vars := varRe.FindAllStringSubmatch(t.Body, -1)
		for i, v := range vars {
			if v[1] != fmt.Sprint(i+1) {
				return fmt.Errorf("pack %s: template %s: variables must be {{1}}, {{2}}, … in order", p.ID, t.Name)
			}
		}
		if len(vars) != len(t.Samples) {
			return fmt.Errorf("pack %s: template %s: %d variables but %d samples", p.ID, t.Name, len(vars), len(t.Samples))
		}
		if len(t.Buttons) > 3 || len(t.Body) > 1024 {
			return fmt.Errorf("pack %s: template %s: at most 3 buttons and 1024 characters", p.ID, t.Name)
		}
		names[t.Name] = true
	}
	for _, r := range p.Reminders {
		if !names[r.Template] {
			return fmt.Errorf("pack %s: rule %q uses template %q, which isn't in the pack", p.ID, r.Name, r.Template)
		}
	}
	for _, s := range p.Segments {
		if s.Name == "" || !json.Valid(s.Rules) {
			return fmt.Errorf("pack %s: segment %q needs a name and rules", p.ID, s.Name)
		}
	}
	return nil
}

// All returns every pack, sorted by id. It panics on a bad pack file, which
// the tests catch before anything ships.
func All() []Pack {
	entries, err := dataFS.ReadDir("data")
	if err != nil {
		panic(err)
	}
	var out []Pack
	for _, e := range entries {
		b, err := dataFS.ReadFile("data/" + e.Name())
		if err != nil {
			panic(err)
		}
		var p Pack
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			panic(fmt.Sprintf("pack %s: %v", e.Name(), err))
		}
		if err := p.validate(); err != nil {
			panic(err)
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func byID(id string) (Pack, bool) {
	for _, p := range All() {
		if p.ID == id {
			return p, true
		}
	}
	return Pack{}, false
}

// ForVertical is the pack that fits an onboarding vertical ("" when none).
func ForVertical(v string) string {
	for _, p := range All() {
		for _, x := range p.Verticals {
			if x == v {
				return p.ID
			}
		}
	}
	return ""
}
