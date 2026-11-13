package onboarding

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/modules/booking"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/internal/modules/practice"
	"github.com/zerodha/fastglue"
)

// Environment variables (set in the host, never committed).
const (
	// SignupEnv opens self-serve sign-up when "open". Off by default, so a
	// staging or single-clinic instance never takes strangers' sign-ups.
	SignupEnv = "WELINE_SIGNUP"
	// SourceURLEnv is where the AGPL source of the running build is (ADR-005);
	// defaults to the public repository.
	SourceURLEnv = "WELINE_SOURCE_URL"
	// TermsURLEnv and PrivacyURLEnv link the sign-up page to the legal pages.
	TermsURLEnv   = "WELINE_TERMS_URL"
	PrivacyURLEnv = "WELINE_PRIVACY_URL"

	defaultSourceURL = "https://github.com/irajput215/weline"
	pagePath         = "/x/onboarding/signup" // the public page (outside /api)
	signupPath       = "/x/onboarding/public/signup"
	sourcePath       = "/x/source" // AGPL §13: link to the running source
	signupsPerHour   = 5           // per client IP
	pageReadsPerMin  = 60
)

// BuildCommit is set at build time (-ldflags "-X ...onboarding.BuildCommit=<sha>")
// so /x/source points at the exact source running.
var BuildCommit = ""

//go:embed page.html
var pageHTML string

var pageTmpl = template.Must(template.New("signup").Parse(pageHTML))

type pageData struct {
	API, LoginURL, SourceURL, TermsURL, PrivacyURL string
	SiteKey, Nonce, DefaultTZ                      string
	TimeZones                                      []string
}

// limiter counts hits per key in a fixed window.
type limiter interface {
	allow(ctx context.Context, key string, max int, window time.Duration) bool
}

type redisLimiter struct{ rdb *redis.Client }

func (l redisLimiter) allow(ctx context.Context, key string, max int, window time.Duration) bool {
	k := "wl:onboarding:rl:" + key
	n, err := l.rdb.Incr(ctx, k).Result()
	if err != nil {
		return true // fail open like upstream's limiter; the captcha still applies
	}
	if n == 1 {
		l.rdb.Expire(ctx, k, window)
	}
	return n <= int64(max)
}

type memoryLimiter struct {
	mu sync.Mutex
	m  map[string]int
}

func (l *memoryLimiter) allow(_ context.Context, key string, max int, _ time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[string]int{}
	}
	l.m[key]++
	return l.m[key] <= max
}

type public struct {
	svc        *Service
	captcha    booking.Captcha
	limit      limiter
	trustProxy bool
	basePath   string
	open       bool
}

func signupOpen() bool { return strings.EqualFold(os.Getenv(SignupEnv), "open") }

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

// SourceURL is the AGPL source link for the running build.
func SourceURL() string {
	base := strings.TrimSuffix(envOr(SourceURLEnv, defaultSourceURL), "/")
	if BuildCommit != "" && strings.HasPrefix(base, "https://github.com/") {
		return base + "/tree/" + BuildCommit
	}
	return base
}

func (p *public) clientIP(r *fastglue.Request) string {
	if p.trustProxy {
		if xff := string(r.RequestCtx.Request.Header.Peek("X-Forwarded-For")); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RequestCtx.RemoteAddr().String())
	if err != nil {
		return r.RequestCtx.RemoteAddr().String()
	}
	return host
}

func (p *public) allow(r *fastglue.Request, bucket string, max int, window time.Duration) bool {
	ctx, cancel := kit.Context()
	defer cancel()
	h := sha256.Sum256([]byte("wl-onboarding:" + p.clientIP(r)))
	return p.limit.allow(ctx, bucket+":"+hex.EncodeToString(h[:12]), max, window)
}

var (
	errSlowDown = kit.TooMany("too many sign-ups from here; please try again later")
	errClosed   = kit.NotFound("sign-up isn't open on this server")
)

func nonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// page serves the public sign-up page.
func (p *public) page(r *fastglue.Request) error {
	if !p.allow(r, "page", pageReadsPerMin, time.Minute) {
		return kit.SendError(r, errSlowDown)
	}
	h := &r.RequestCtx.Response.Header
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	if !p.open {
		r.RequestCtx.SetStatusCode(404)
		r.RequestCtx.SetBodyString(`<!doctype html><meta charset="utf-8"><title>Sign-up closed</title><p>Sign-up isn't open on this server. Ask your organization's admin to add you.</p>`)
		return nil
	}
	n := nonce()
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+n+"' https://challenges.cloudflare.com; "+
		"frame-src https://challenges.cloudflare.com; connect-src 'self'; style-src 'unsafe-inline'; img-src 'self' data:; "+
		"base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, pageData{API: p.basePath + signupPath, LoginURL: p.basePath + "/login", SourceURL: p.basePath + sourcePath,
		TermsURL: envOr(TermsURLEnv, "#terms"), PrivacyURL: envOr(PrivacyURLEnv, "#privacy"), SiteKey: p.captcha.SiteKey(), Nonce: n,
		DefaultTZ: practice.DefaultTimeZone, TimeZones: practice.TimeZones}); err != nil {
		return kit.SendError(r, err)
	}
	r.RequestCtx.SetBody(buf.Bytes())
	return nil
}

// signup handles the form (R1, R2).
func (p *public) signup(r *fastglue.Request) error {
	if !p.open {
		return kit.SendError(r, errClosed)
	}
	if !p.allow(r, "signup", signupsPerHour, time.Hour) {
		return kit.SendError(r, errSlowDown)
	}
	var in SignupInput
	if err := json.Unmarshal(r.RequestCtx.PostBody(), &in); err != nil {
		return kit.SendError(r, kit.Invalid("invalid request body"))
	}
	ctx, cancel := kit.Context()
	defer cancel()
	if p.captcha.SiteKey() == "" {
		return kit.SendError(r, kit.NotReady("sign-up isn't set up yet (no robot check configured)"))
	}
	if err := p.captcha.Verify(ctx, in.CaptchaToken, p.clientIP(r)); err != nil {
		return kit.SendError(r, err)
	}
	res, err := p.svc.Signup(ctx, in)
	if err != nil && res == nil {
		return kit.SendError(r, err)
	}
	return r.SendEnvelope(res)
}

// source redirects to the AGPL source of the running build (ADR-005).
func (p *public) source(r *fastglue.Request) error {
	r.RequestCtx.Redirect(SourceURL(), 302)
	return nil
}
