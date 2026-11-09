package booking

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"html/template"
	"time"

	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/zerodha/fastglue"
)

// The public booking page is served by the module rather than the Vue app:
// module Vue routes live inside the logged-in layout, and a public one would
// need another seam in router/index.ts (design.md, "Public booking page").

//go:embed page.html
var pageHTML string

var pageTmpl = template.Must(template.New("book").Parse(pageHTML))

type pageData struct {
	Clinic  string
	API     string // JSON-escaped in the script by html/template
	SiteKey string
	Nonce   string
}

func nonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// page serves GET /book/{slug}.
func (p *public) page(basePath string) fastglue.FastRequestHandler {
	return func(r *fastglue.Request) error {
		if !p.allow(r, "read", publicReadsPerMinute, time.Minute) {
			return kit.SendError(r, errSlowDown)
		}
		ctx, cancel := kit.Context()
		defer cancel()
		slug := slugOf(r)
		prof, err := p.svc.orgBySlug(ctx, slug)
		h := &r.RequestCtx.Response.Header
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-store")
		if err != nil {
			r.RequestCtx.SetStatusCode(404)
			r.RequestCtx.SetBodyString(`<!doctype html><meta charset="utf-8"><title>Not found</title><p>Booking page not found.</p>`)
			return nil
		}
		n := nonce()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+n+"' https://challenges.cloudflare.com; "+
			"frame-src https://challenges.cloudflare.com; connect-src 'self'; style-src 'unsafe-inline'; img-src 'self' data:; "+
			"base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		var buf bytes.Buffer
		if err := pageTmpl.Execute(&buf, pageData{Clinic: prof.Name, API: basePath + publicBase + "/" + prof.BookingSlug,
			SiteKey: p.captcha.SiteKey(), Nonce: n}); err != nil {
			return kit.SendError(r, err)
		}
		r.RequestCtx.SetBody(buf.Bytes())
		return nil
	}
}
