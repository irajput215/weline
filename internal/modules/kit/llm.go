package kit

import "context"

// LLM is a language model behind Weline's AI provider settings (Phase 4).
// Implementations live in the ai module; tests use a scripted one.
type LLM interface {
	Complete(ctx context.Context, req LLMRequest) (*LLMResponse, error)
}

// LLMMessage is one turn of a conversation.
type LLMMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// LLMRequest asks the model for one reply.
type LLMRequest struct {
	System      string
	Messages    []LLMMessage
	MaxTokens   int
	Temperature float64
}

// LLMResponse is the model's reply and what it cost.
type LLMResponse struct {
	Text      string
	TokensIn  int
	TokensOut int
}
