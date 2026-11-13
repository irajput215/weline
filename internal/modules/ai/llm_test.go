package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shridarpatil/whatomate/internal/crypto"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMClients(t *testing.T) {
	var got struct {
		path    string
		headers http.Header
		body    map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path, got.headers = r.URL.Path, r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got.body)
		switch {
		case strings.Contains(r.URL.Path, "anthropic"):
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi from claude"}],"usage":{"input_tokens":12,"output_tokens":3}}`))
		case strings.Contains(r.URL.Path, "openai"):
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi from gpt"}}],"usage":{"prompt_tokens":11,"completion_tokens":4}}`))
		case strings.Contains(r.URL.Path, "fail"):
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"bad key sk-secret"}`))
		default:
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hi from gemini"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`))
		}
	}))
	defer srv.Close()
	ep := endpoints{Anthropic: srv.URL + "/anthropic", OpenAI: srv.URL + "/openai", Google: srv.URL + "/google/%s"}
	req := kit.LLMRequest{System: "be nice", Messages: []kit.LLMMessage{{Role: "user", Content: "hello"}}, MaxTokens: 50}

	l, err := newLLM("anthropic", "claude-sonnet-5-5", "sk-ant", 0, srv.Client(), ep)
	require.NoError(t, err)
	r, err := l.Complete(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "hi from claude", r.Text)
	assert.Equal(t, 12, r.TokensIn)
	assert.Equal(t, "sk-ant", got.headers.Get("x-api-key"))
	assert.Equal(t, "2023-06-01", got.headers.Get("anthropic-version"))
	assert.Equal(t, "be nice", got.body["system"])
	assert.Equal(t, float64(50), got.body["max_tokens"])

	l, _ = newLLM("openai", "gpt-x", "sk-oa", 0, srv.Client(), ep)
	r, err = l.Complete(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "hi from gpt", r.Text)
	assert.Equal(t, "Bearer sk-oa", got.headers.Get("Authorization"))
	assert.Equal(t, "system", got.body["messages"].([]any)[0].(map[string]any)["role"])

	l, _ = newLLM("google", "gemini-x", "g-key", 0, srv.Client(), ep)
	r, err = l.Complete(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "hi from gemini", r.Text)
	assert.Equal(t, "/google/gemini-x", got.path)
	assert.Equal(t, "g-key", got.headers.Get("x-goog-api-key"), "key in a header, not the URL")

	l, _ = newLLM("anthropic", "m", "sk-secret", 0, srv.Client(), endpoints{Anthropic: srv.URL + "/fail"})
	_, err = l.Complete(ctx, req)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sk-secret", "errors never echo the key or the body")

	_, err = newLLM("anthropic", "", "k", 0, nil, ep)
	assert.ErrorIs(t, err, ErrNoProvider)
}

func TestNewLLM_UsesWelineSettings(t *testing.T) {
	db := setupDB(t)
	org := testutil.CreateTestOrganization(t, db)
	_, err := NewLLM(ctx, db, "enc-key", org.ID, nil, defaultEndpoints)
	assert.ErrorIs(t, err, ErrNoProvider, "no AI settings yet")

	enc, err := crypto.Encrypt("sk-real", "enc-key")
	require.NoError(t, err)
	cs := models.ChatbotSettings{OrganizationID: org.ID}
	cs.AI = models.AIConfig{Enabled: true, Provider: models.AIProviderAnthropic, APIKey: enc, Model: "claude-sonnet-5-5", MaxTokens: 400}
	require.NoError(t, db.Create(&cs).Error)
	info, err := Info(ctx, db, org.ID)
	require.NoError(t, err)
	assert.Equal(t, ProviderInfo{Name: "anthropic", Model: "claude-sonnet-5-5", KeySet: true}, *info)
	l, err := NewLLM(ctx, db, "enc-key", org.ID, nil, defaultEndpoints)
	require.NoError(t, err)
	a := l.(*anthropicLLM)
	assert.Equal(t, "sk-real", a.key, "decrypted with Weline's key")
	assert.Equal(t, 400, a.maxTokens)
}
