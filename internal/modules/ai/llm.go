package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/crypto"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/modules/kit"
	"gorm.io/gorm"
)

const llmTimeout = 20 * time.Second

// ErrNoProvider means Weline's AI provider settings have no key or model.
var ErrNoProvider = kit.NotReady("set an AI provider, model and key in Settings → Chatbot → AI first")

// ProviderInfo is what the settings page shows about the AI provider (R1).
type ProviderInfo struct {
	Name   string `json:"name"`
	Model  string `json:"model"`
	KeySet bool   `json:"key_set"`
}

// endpoints can be pointed at test servers.
type endpoints struct {
	Anthropic, OpenAI, Google string
}

var defaultEndpoints = endpoints{
	Anthropic: "https://api.anthropic.com/v1/messages",
	OpenAI:    "https://api.openai.com/v1/chat/completions",
	Google:    "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
}

// providerSettings reads the organization's AI settings from Weline's chatbot
// settings (the organization-level row).
func providerSettings(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (*models.AIConfig, error) {
	var rows []models.ChatbotSettings
	if err := db.WithContext(ctx).Where("organization_id = ? AND whats_app_account = ''", orgID).Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load AI settings: %w", err)
	}
	if len(rows) == 0 {
		return &models.AIConfig{}, nil
	}
	return &rows[0].AI, nil
}

// Info describes the AI provider without the key.
func Info(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (*ProviderInfo, error) {
	c, err := providerSettings(ctx, db, orgID)
	if err != nil {
		return nil, err
	}
	return &ProviderInfo{Name: string(c.Provider), Model: c.Model, KeySet: c.APIKey != ""}, nil
}

// NewLLM builds the organization's model client from Weline's settings,
// decrypting the key with the encryption key (R1).
func NewLLM(ctx context.Context, db *gorm.DB, encryptionKey string, orgID uuid.UUID, hc *http.Client, ep endpoints) (kit.LLM, error) {
	c, err := providerSettings(ctx, db, orgID)
	if err != nil {
		return nil, err
	}
	if c.APIKey == "" || c.Model == "" {
		return nil, ErrNoProvider
	}
	key, err := crypto.Decrypt(c.APIKey, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt AI key: %w", err)
	}
	return newLLM(string(c.Provider), c.Model, key, c.MaxTokens, hc, ep)
}

// newLLM builds a client for a provider, model and plain key.
func newLLM(prov, model, key string, maxTokens int, hc *http.Client, ep endpoints) (kit.LLM, error) {
	if key == "" || model == "" {
		return nil, ErrNoProvider
	}
	if hc == nil {
		hc = &http.Client{Timeout: llmTimeout}
	}
	base := client{key: key, model: model, http: hc, maxTokens: maxTokens}
	switch models.AIProvider(prov) {
	case models.AIProviderAnthropic:
		return &anthropicLLM{client: base, url: ep.Anthropic}, nil
	case models.AIProviderOpenAI:
		return &openaiLLM{client: base, url: ep.OpenAI}, nil
	case models.AIProviderGoogle:
		return &googleLLM{client: base, url: fmt.Sprintf(ep.Google, model)}, nil
	}
	return nil, ErrNoProvider
}

type client struct {
	key, model string
	http       *http.Client
	maxTokens  int
}

func (c client) tokens(req kit.LLMRequest) int {
	if req.MaxTokens > 0 {
		return req.MaxTokens
	}
	if c.maxTokens > 0 {
		return c.maxTokens
	}
	return 500
}

// post sends JSON and decodes the answer; errors never include the key or the
// response body (which may echo the prompt).
func (c client) post(ctx context.Context, url string, headers map[string]string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("AI provider unreachable")
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read AI provider answer")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("AI provider answered %d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode AI provider answer")
	}
	return nil
}

type anthropicLLM struct {
	client
	url string
}

func (a *anthropicLLM) Complete(ctx context.Context, req kit.LLMRequest) (*kit.LLMResponse, error) {
	msgs := make([]map[string]string, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			In  int `json:"input_tokens"`
			Out int `json:"output_tokens"`
		} `json:"usage"`
	}
	body := map[string]any{"model": a.model, "max_tokens": a.tokens(req), "system": req.System, "messages": msgs, "temperature": req.Temperature}
	if err := a.post(ctx, a.url, map[string]string{"x-api-key": a.key, "anthropic-version": "2023-06-01"}, body, &out); err != nil {
		return nil, err
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return &kit.LLMResponse{Text: sb.String(), TokensIn: out.Usage.In, TokensOut: out.Usage.Out}, nil
}

type openaiLLM struct {
	client
	url string
}

func (o *openaiLLM) Complete(ctx context.Context, req kit.LLMRequest) (*kit.LLMResponse, error) {
	msgs := []map[string]string{{"role": "system", "content": req.System}}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			In  int `json:"prompt_tokens"`
			Out int `json:"completion_tokens"`
		} `json:"usage"`
	}
	body := map[string]any{"model": o.model, "max_tokens": o.tokens(req), "messages": msgs, "temperature": req.Temperature}
	if err := o.post(ctx, o.url, map[string]string{"Authorization": "Bearer " + o.key}, body, &out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("AI provider gave no answer")
	}
	return &kit.LLMResponse{Text: out.Choices[0].Message.Content, TokensIn: out.Usage.In, TokensOut: out.Usage.Out}, nil
}

type googleLLM struct {
	client
	url string
}

func (g *googleLLM) Complete(ctx context.Context, req kit.LLMRequest) (*kit.LLMResponse, error) {
	contents := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, map[string]any{"role": role, "parts": []map[string]string{{"text": m.Content}}})
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Usage struct {
			In  int `json:"promptTokenCount"`
			Out int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	body := map[string]any{"systemInstruction": map[string]any{"parts": []map[string]string{{"text": req.System}}}, "contents": contents,
		"generationConfig": map[string]any{"maxOutputTokens": g.tokens(req), "temperature": req.Temperature}}
	if err := g.post(ctx, g.url, map[string]string{"x-goog-api-key": g.key}, body, &out); err != nil {
		return nil, err
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("AI provider gave no answer")
	}
	return &kit.LLMResponse{Text: out.Candidates[0].Content.Parts[0].Text, TokensIn: out.Usage.In, TokensOut: out.Usage.Out}, nil
}
