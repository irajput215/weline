package billing

import (
	"os"
	"strings"

	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// Plan is a set of limits (ADR-005; prices live in Stripe).
type Plan struct {
	Key    string         `json:"key"`
	Name   string         `json:"name"`
	Limits map[string]int `json:"limits"`
	// PriceEnv names the environment variable with the plan's Stripe price id.
	PriceEnv string `json:"-"`
}

const (
	PlanTrial   = "trial"
	PlanStarter = "starter"
	PlanClinic  = "clinic"
	PlanGroup   = "group"
	planNone    = "none" // trial over or subscription ended: no metered work
)

func limits(users, contacts, broadcast, ai, systems, numbers int) map[string]int {
	return map[string]int{kit.MetricUsers: users, kit.MetricContacts: contacts, kit.MetricBroadcast: broadcast,
		kit.MetricAIRuns: ai, kit.MetricConnectors: systems, kit.MetricNumbers: numbers}
}

// Plans in order, cheapest first.
var Plans = []Plan{
	{Key: PlanTrial, Name: "Trial", Limits: limits(3, 500, 200, 200, 1, 1)},
	{Key: PlanStarter, Name: "Starter", Limits: limits(3, 2000, 2000, 1000, 1, 1), PriceEnv: "WELINE_STRIPE_PRICE_STARTER"},
	{Key: PlanClinic, Name: "Clinic", Limits: limits(10, 10000, 10000, 5000, 1, 2), PriceEnv: "WELINE_STRIPE_PRICE_CLINIC"},
	{Key: PlanGroup, Name: "Group", Limits: limits(30, 50000, 50000, 20000, 3, 5), PriceEnv: "WELINE_STRIPE_PRICE_GROUP"},
}

var nonePlan = Plan{Key: planNone, Name: "No plan", Limits: limits(3, 500, 0, 0, 1, 1)}

// Enforced metrics are checked before the work (kit.Limits); the others are
// created on upstream screens Weline doesn't change, so they're shown and
// warned about, not blocked.
var Enforced = map[string]bool{kit.MetricBroadcast: true, kit.MetricAIRuns: true}

// Monthly metrics reset each calendar month (UTC).
var Monthly = map[string]bool{kit.MetricBroadcast: true, kit.MetricAIRuns: true}

// MetricOrder is how usage is listed.
var MetricOrder = []string{kit.MetricBroadcast, kit.MetricAIRuns, kit.MetricContacts, kit.MetricUsers, kit.MetricNumbers, kit.MetricConnectors}

func planByKey(k string) (Plan, bool) {
	for _, p := range Plans {
		if p.Key == k {
			return p, true
		}
	}
	return Plan{}, false
}

// priceID is the Stripe price for a paid plan, from the environment.
func (p Plan) priceID() string {
	if p.PriceEnv == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(p.PriceEnv))
}

// planForPrice maps a Stripe price back to a plan.
func planForPrice(price string) (Plan, bool) {
	for _, p := range Plans {
		if id := p.priceID(); id != "" && id == price {
			return p, true
		}
	}
	return Plan{}, false
}
