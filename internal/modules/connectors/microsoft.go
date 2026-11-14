package connectors

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/msgraph"
	"github.com/shridarpatil/whatomate/internal/modules/connectors/provider"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
)

// Environment for Weline's Microsoft Entra app (set as secrets in the host).
const (
	MicrosoftClientIDEnv     = "WELINE_MS_CLIENT_ID"
	MicrosoftClientSecretEnv = "WELINE_MS_CLIENT_SECRET"
	consentTTL               = 30 * time.Minute
	consentPath              = "/x/connectors/microsoft/consent" // Microsoft sends the admin back here (outside /api)
)

// MicrosoftOptions configure Microsoft 365; tests point Login and API at a fake.
type MicrosoftOptions struct {
	App        msgraph.App
	Login, API string
	HTTP       *http.Client
	PublicURL  string // Weline's public address, for the consent redirect
}

// MicrosoftOptionsFromEnv reads the app and public address from the environment.
func MicrosoftOptionsFromEnv(publicURL string) MicrosoftOptions {
	return MicrosoftOptions{App: msgraph.App{ClientID: os.Getenv(MicrosoftClientIDEnv), ClientSecret: os.Getenv(MicrosoftClientSecretEnv)},
		PublicURL: strings.TrimSuffix(publicURL, "/")}
}

func tenantKey(t string) string { return "tenant:" + strings.ToLower(t) }

// MicrosoftFactory is Microsoft 365 as a provider (step 7.2). Without Weline's
// app it's listed as not available. A tenant must have been approved through
// this organization's consent link before it can be connected.
func (s *Service) MicrosoftFactory(b *booking.Service, opt MicrosoftOptions) Factory {
	fields := []Field{{Name: "tenant_id"}, {Name: "mailbox"}}
	c, err := msgraph.NewClient(opt.App, msgraph.Options{Login: opt.Login, API: opt.API, HTTP: opt.HTTP})
	if err != nil {
		return Factory{Name: msgraph.Name, Available: false, Fields: fields}
	}
	s.ms, s.msRedirect = c, opt.PublicURL+consentPath
	build := func(orgID uuid.UUID, creds map[string]string, raw json.RawMessage) (*msgraph.Provider, error) {
		set, err := msgraph.ParseSettings(creds["tenant_id"], creds["mailbox"], raw)
		if err != nil {
			return nil, err
		}
		return msgraph.New(c, b.Provider(orgID), set), nil
	}
	return Factory{Name: msgraph.Name, Available: true, Fields: fields, Consent: true,
		Build: func(orgID uuid.UUID, creds map[string]string, raw json.RawMessage) (provider.Provider, error) {
			return build(orgID, creds, raw)
		},
		precheck: func(ctx context.Context, orgID uuid.UUID, creds map[string]string) error {
			t := strings.TrimSpace(creds["tenant_id"])
			if !msgraph.ValidTenant(t) {
				return kit.Invalid("tenant_id must be your Microsoft 365 tenant id; use Approve Weline in Microsoft 365 to fill it in")
			}
			var owner struct{ OrganizationID uuid.UUID }
			if err := s.db.WithContext(ctx).Raw(`SELECT organization_id FROM wl.connector_claims WHERE provider = ? AND key_hash = ?`,
				msgraph.Name, claimHash(msgraph.Name, tenantKey(t))).Scan(&owner).Error; err != nil {
				return err
			}
			if owner.OrganizationID != orgID {
				return kit.Invalid("approve Weline in Microsoft 365 first (Approve Weline in Microsoft 365), signed in as your Microsoft 365 admin")
			}
			return nil
		},
		claims: func(creds map[string]string, raw json.RawMessage) ([]string, error) {
			set, err := msgraph.ParseSettings(creds["tenant_id"], creds["mailbox"], raw)
			if err != nil {
				return nil, err
			}
			keys := []string{tenantKey(set.Tenant)} // kept: it was claimed by the consent
			for _, m := range set.Mailboxes() {
				keys = append(keys, "mailbox:"+m)
			}
			return keys, nil
		},
		Test: func(ctx context.Context, creds map[string]string, raw json.RawMessage) (string, error) {
			p, err := build(uuid.Nil, creds, raw)
			if err != nil {
				return "", err
			}
			return p.TestConnection(ctx)
		}}
}

func (s *Service) signState(orgID uuid.UUID, exp int64) string {
	m := hmac.New(sha256.New, []byte("weline-ms-consent:"+s.key))
	m.Write([]byte(orgID.String() + "." + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(m.Sum(nil))
}

// ConsentURL is the admin-consent link for an organization: its state is
// signed with Weline's key and expires in 30 minutes.
func (s *Service) ConsentURL(orgID uuid.UUID) (string, error) {
	if s.ms == nil {
		return "", kit.NotReady("Microsoft 365 isn't set up on this server")
	}
	if s.key == "" || strings.HasPrefix(s.msRedirect, consentPath) {
		return "", kit.NotReady("Microsoft 365 needs Weline's public address (WELINE_PUBLIC_URL) and encryption key")
	}
	exp := s.now().Add(consentTTL).Unix()
	state := fmt.Sprintf("%s.%d.%s", orgID, exp, s.signState(orgID, exp))
	return s.ms.ConsentURL(s.msRedirect, state), nil
}

var errBadConsent = kit.Invalid("this approval link is invalid or expired; start again from Connected system")

// CompleteConsent records an approved tenant as the organization's, after
// checking the state Weline signed. A tenant belongs to one organization.
func (s *Service) CompleteConsent(ctx context.Context, state, tenant string) (uuid.UUID, error) {
	parts := strings.Split(state, ".")
	if len(parts) != 3 || s.key == "" {
		return uuid.Nil, errBadConsent
	}
	orgID, err := uuid.Parse(parts[0])
	exp, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || err2 != nil || s.now().Unix() > exp || !hmac.Equal([]byte(parts[2]), []byte(s.signState(orgID, exp))) {
		return uuid.Nil, errBadConsent
	}
	if !msgraph.ValidTenant(tenant) {
		return uuid.Nil, kit.Invalid("Microsoft didn't say which tenant approved Weline")
	}
	h := claimHash(msgraph.Name, tenantKey(tenant))
	if err := s.db.WithContext(ctx).Exec(`INSERT INTO wl.connector_claims (provider, key_hash, organization_id) VALUES (?, ?, ?)
		ON CONFLICT (provider, key_hash) DO NOTHING`, msgraph.Name, h, orgID).Error; err != nil {
		return uuid.Nil, err
	}
	var owner struct{ OrganizationID uuid.UUID }
	if err := s.db.WithContext(ctx).Raw(`SELECT organization_id FROM wl.connector_claims WHERE provider = ? AND key_hash = ?`, msgraph.Name, h).Scan(&owner).Error; err != nil {
		return uuid.Nil, err
	}
	if owner.OrganizationID != orgID {
		return uuid.Nil, kit.Conflict("this Microsoft 365 tenant is already connected to another Weline account; contact Weline support")
	}
	return orgID, nil
}
