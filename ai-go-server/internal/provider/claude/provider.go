// Package claude adapts Anthropic's Messages API to the normalized LLM API.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/llm"
	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/transport"
)

const defaultBaseURL = "https://api.anthropic.com/v1"

// Config configures an Anthropic Messages API adapter. Client is optional and
// allows tests to use the same production transport behavior.
type Config struct {
	BaseURL string
	APIKey  string
	Client  *transport.Client
}

// Provider translates between the normalized request and Anthropic's protocol.
type Provider struct {
	baseURL string
	apiKey  string
	client  *transport.Client
}

// New creates a provider for Anthropic's public endpoint.
func New(apiKey string) *Provider {
	return NewWithConfig(Config{APIKey: apiKey})
}

// NewWithConfig creates a provider with an overridable endpoint for testing.
func NewWithConfig(config Config) *Provider {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	client := config.Client
	if client == nil {
		client = transport.NewClient()
	}
	return &Provider{baseURL: baseURL, apiKey: strings.TrimSpace(config.APIKey), client: client}
}

func (p *Provider) Name() string {
	return "claude"
}

func (p *Provider) Capabilities() llm.Capability {
	return llm.Capability{Streaming: true}
}

// Chat sends a normalized request to Anthropic's Messages API.
func (p *Provider) Chat(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	request.Stream = false
	response, err := p.doRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("claude: decode response: %w", err)
	}

	var content strings.Builder
	for _, block := range result.Content {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}
	return &llm.ChatResponse{
		Content:      content.String(),
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
	}, nil
}

// ChatStream converts Anthropic SSE events to normalized content chunks.
func (p *Provider) ChatStream(ctx context.Context, request llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	request.Stream = true
	response, err := p.doRequest(ctx, request)
	if err != nil {
		return nil, err
	}

	chunks := make(chan llm.StreamChunk)
	go func() {
		defer close(chunks)
		defer response.Body.Close()

		sawDone := false
		streamErr := transport.ParseSSE(response.Body, func(data []byte) error {
			delta, done, err := parseDelta(data)
			if err != nil {
				return err
			}
			if done {
				sawDone = true
				return nil
			}
			if delta == "" {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case chunks <- llm.StreamChunk{Content: delta}:
				return nil
			}
		})
		if streamErr == nil && !sawDone {
			streamErr = fmt.Errorf("claude: stream ended before message_stop")
		}
		if streamErr == nil {
			return
		}
		select {
		case <-ctx.Done():
		case chunks <- llm.StreamChunk{Err: streamErr}:
		}
	}()

	return chunks, nil
}

func (p *Provider) doRequest(ctx context.Context, request llm.ChatRequest) (*http.Response, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("claude provider is not initialized")
	}
	body, err := p.adaptRequest(request)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("claude: encode request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/messages",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("claude: build request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("anthropic-version", "2023-06-01")
	if p.apiKey != "" {
		httpRequest.Header.Set("x-api-key", p.apiKey)
	}

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("claude: send request: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		return nil, fmt.Errorf("claude: %s", apiError(response))
	}
	return response, nil
}

// adaptRequest maps system messages and Anthropic-required fields explicitly.
func (p *Provider) adaptRequest(request llm.ChatRequest) (map[string]any, error) {
	if request.MaxTokens < 0 {
		return nil, fmt.Errorf("claude: max tokens must not be negative")
	}

	var systemParts []string
	messages := make([]map[string]string, 0, len(request.Messages))
	for _, message := range request.Messages {
		switch message.Role {
		case llm.RoleSystem:
			systemParts = append(systemParts, message.Content)
		case llm.RoleUser, llm.RoleAssistant:
			messages = append(messages, map[string]string{
				"role":    string(message.Role),
				"content": message.Content,
			})
		default:
			return nil, fmt.Errorf("claude: role %q is not supported by the M02 adapter", message.Role)
		}
	}

	maxTokens := request.MaxTokens
	if maxTokens == 0 {
		maxTokens = 1024
	}
	body := map[string]any{
		"model":      request.Model,
		"messages":   messages,
		"max_tokens": maxTokens,
	}
	if system := strings.Join(systemParts, "\n"); system != "" {
		body["system"] = system
	}
	if request.Temperature != nil {
		body["temperature"] = *request.Temperature
	}
	if request.Stream {
		body["stream"] = true
	}
	return body, nil
}

func parseDelta(data []byte) (delta string, done bool, err error) {
	var event struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return "", false, fmt.Errorf("decode Claude stream event: %w", err)
	}
	if event.Type == "message_stop" {
		return "", true, nil
	}
	if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
		return event.Delta.Text, false, nil
	}
	return "", false, nil
}

func apiError(response *http.Response) string {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Error.Message != "" {
		return response.Status + ": " + payload.Error.Message
	}
	if message := strings.TrimSpace(string(body)); message != "" {
		return response.Status + ": " + message
	}
	return response.Status
}
