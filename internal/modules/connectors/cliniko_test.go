package connectors

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// miniCliniko is a small Cliniko for wiring tests: one business, one
// practitioner, one appointment type, two patients. The cliniko package's own
// tests cover the API in depth.
type miniCliniko struct {
	srv    *httptest.Server
	mu     sync.Mutex
	booked []map[string]any
	keyOK  string
}

func newMini(t *testing.T) *miniCliniko {
	m := &miniCliniko{keyOK: "goodkey-au1"}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		user, _, _ := r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		if user != m.keyOK {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
			return
		}
		base := m.srv.URL + "/v1"
		self := func(kind, id string) map[string]any {
			return map[string]any{"links": map[string]any{"self": base + "/" + kind + "/" + id}}
		}
		out := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		p := strings.TrimPrefix(r.URL.Path, "/v1")
		switch {
		case p == "/user":
			out(map[string]any{"first_name": "Weline", "last_name": "Integration"})
		case p == "/businesses":
			out(map[string]any{"businesses": []any{map[string]any{"id": "11", "display_name": "Harbour St", "time_zone_identifier": "Australia/Sydney"}}, "links": map[string]any{}})
		case p == "/businesses/11/practitioners":
			out(map[string]any{"practitioners": []any{map[string]any{"id": "31", "display_name": "Sam Lee", "active": true, "show_in_online_bookings": true}}, "links": map[string]any{}})
		case p == "/practitioners":
			out(map[string]any{"practitioners": []any{map[string]any{"id": "31", "display_name": "Sam Lee"}}, "links": map[string]any{}})
		case p == "/appointment_types":
			out(map[string]any{"appointment_types": []any{map[string]any{"id": "21", "name": "Initial physio", "duration_in_minutes": 45, "max_attendees": 1, "show_in_online_bookings": true}}, "links": map[string]any{}})
		case strings.HasSuffix(p, "/available_times"):
			from, _ := time.Parse("2006-01-02", r.URL.Query().Get("from"))
			t := time.Date(from.Year(), from.Month(), from.Day(), 23, 0, 0, 0, time.UTC) // 10:00 next day in Sydney
			out(map[string]any{"available_times": []any{map[string]any{"appointment_start": t.Format(time.RFC3339)}}, "links": map[string]any{}})
		case p == "/patients":
			out(map[string]any{"patients": []any{
				map[string]any{"id": "201", "first_name": "Pat", "last_name": "Kim", "updated_at": "2026-11-01T00:00:00Z",
					"patient_phone_numbers": []any{map[string]any{"number": "0412 345 678", "phone_type": "Mobile"}}},
				map[string]any{"id": "202", "first_name": "Ola", "last_name": "Fry", "updated_at": "2026-11-02T00:00:00Z",
					"patient_phone_numbers": []any{map[string]any{"number": "0400 000 001", "phone_type": "Mobile"}}},
			}, "links": map[string]any{}})
		case p == "/patients/201":
			out(map[string]any{"id": "201", "first_name": "Pat", "last_name": "Kim",
				"patient_phone_numbers": []any{map[string]any{"number": "0412 345 678", "phone_type": "Mobile"}}})
		case p == "/individual_appointments" && r.Method == http.MethodPost:
			var body map[string]any
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			m.booked = append(m.booked, body)
			start, _ := time.Parse(time.RFC3339, body["starts_at"].(string))
			out(map[string]any{"id": "901", "starts_at": start.Format(time.RFC3339), "ends_at": start.Add(45 * time.Minute).Format(time.RFC3339),
				"patient": self("patients", "201"), "practitioner": self("practitioners", "31"),
				"appointment_type": self("appointment_types", "21"), "business": self("businesses", "11")})
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"Not found"}`))
		}
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (f *fixture) withCliniko(t *testing.T, m *miniCliniko) {
	t.Helper()
	f.svc.factories[provider.Cliniko] = f.svc.ClinikoFactory(ClinikoOptions{ContactEmail: "dev@example.test", BaseURL: m.srv.URL + "/v1", HTTP: m.srv.Client()})
}

func TestCliniko_ConnectSyncAndBookOnWhatsApp(t *testing.T) {
	f := newFixture(t)
	m := newMini(t)
	f.withCliniko(t, m)

	_, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "cliniko", Credentials: map[string]string{"api_key": "wrongkey-au1"}})
	assert.True(t, isKind(err, kit.ErrInvalid))
	assert.Contains(t, err.Error(), "Cliniko refused the API key")
	acc, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "cliniko", Credentials: map[string]string{"api_key": "goodkey-au1"}})
	require.NoError(t, err)
	assert.Equal(t, "cliniko", acc.Provider)
	syncs, err := f.svc.Syncs(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, "ok", syncs[0].Outcome)

	// The phone index sync (first run reads every patient).
	n, err := f.svc.SyncAll(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	ids, err := f.svc.Links(f.org, "cliniko").ByPhone(ctx, "61412345678")
	require.NoError(t, err)
	assert.Equal(t, []string{"201"}, ids)
	acc, err = f.svc.Account(ctx, f.org)
	require.NoError(t, err)
	require.NotNil(t, acc.LastSyncAt)
	syncs, err = f.svc.Syncs(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, "sync", syncs[0].Kind)
	assert.JSONEq(t, `{"patients": 2, "phones": 2}`, string(syncs[0].Counts))

	// The WhatsApp flow books into Cliniko through Booking's chatbot.
	bk := booking.NewService(f.db)
	bk.UseProviders(provider.ResolverFunc(f.svc.For))
	contact := testutil.CreateTestContactWith(t, f.db, f.org, testutil.WithPhoneNumber("61412345678"))
	cat, err := bk.Catalog(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, "Initial physio", cat["service_1_label"])
	slots, err := bk.ChatbotSlots(ctx, f.org, booking.ChatbotSlotQuery{OfferID: cat["offer_id"].(string), ServiceLabel: "Initial physio"})
	require.NoError(t, err)
	require.Greater(t, slots["slot_count"], 0)
	label := slots["slot_1_label"].(string)
	assert.Contains(t, label, "10:00 Sam Lee")
	_, confirmation, err := bk.ChatbotBook(ctx, f.org, slots["offer_id"].(string), label, "61412345678")
	require.NoError(t, err)
	assert.Contains(t, confirmation, "Booked: Initial physio with Sam Lee")
	assert.Contains(t, confirmation, "Ref 901")
	require.Len(t, m.booked, 1)
	assert.Equal(t, "201", m.booked[0]["patient_id"], "the patient found through the phone index")
	linked, err := f.svc.Links(f.org, "cliniko").Remote(ctx, "contact", contact.ID)
	require.NoError(t, err)
	assert.Equal(t, "201", linked)
}

func TestCliniko_SyncErrorsAreRecorded(t *testing.T) {
	f := newFixture(t)
	m := newMini(t)
	f.withCliniko(t, m)
	_, err := f.svc.Save(ctx, f.org, f.user, SaveInput{Provider: "cliniko", Credentials: map[string]string{"api_key": "goodkey-au1"}})
	require.NoError(t, err)
	m.mu.Lock()
	m.keyOK = "rotated-au1" // the clinic revoked the key in Cliniko
	m.mu.Unlock()
	_, err = f.svc.SyncAll(context.Background(), nil)
	require.NoError(t, err)
	acc, err := f.svc.Account(ctx, f.org)
	require.NoError(t, err)
	assert.Equal(t, StatusError, acc.Status)
	assert.Contains(t, acc.LastError, "Cliniko refused the API key")
	for _, line := range f.log.lines {
		assert.NotContains(t, line, "goodkey")
	}
}
