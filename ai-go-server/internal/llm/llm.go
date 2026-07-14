// Package llm contains provider-neutral types shared by Agent components.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// Role identifies the author of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one entry in a chat conversation.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the OpenAI-compatible subset shared by common LLM providers.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// ChatResponse is the normalized result returned by a Provider.
type ChatResponse struct {
	Content      string
	InputTokens  int
	OutputTokens int
}

// StreamChunk is the smallest unit emitted by a streaming provider.
// Content and Err are mutually exclusive. An Err chunk terminates the stream.
type StreamChunk struct {
	Content string
	Err     error
}

// Provider is the minimal provider abstraction used by an Agent.
type Provider interface {
	Name() string
	Chat(context.Context, ChatRequest) (*ChatResponse, error)
	ChatStream(context.Context, ChatRequest) (<-chan StreamChunk, error)
}

// ParseInto decodes a structured model response into T.
func ParseInto[T any](raw string) (T, error) {
	var value T
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return value, fmt.Errorf("parse model output as %T: %w", value, err)
	}
	return value, nil
}

// Ptr returns a pointer to value, useful for optional request fields.
func Ptr[T any](value T) *T {
	return &value
}

// DecodeChatResponse translates an OpenAI-compatible response into ChatResponse.
func DecodeChatResponse(r io.Reader) (ChatResponse, error) {
	var raw struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return ChatResponse{}, fmt.Errorf("decode chat response: %w", err)
	}
	if len(raw.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("chat response contains no choices")
	}

	return ChatResponse{
		Content:      raw.Choices[0].Message.Content,
		InputTokens:  raw.Usage.PromptTokens,
		OutputTokens: raw.Usage.CompletionTokens,
	}, nil
}
