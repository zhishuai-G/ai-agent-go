// Package openai implements the OpenAI Chat Completions protocol family.
package openai

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

// Config collects constructor parameters and avoids confusing baseURL with
// APIKey at call sites. Client is optional and primarily useful for tests.
type Config struct {
	Name    string
	BaseURL string
	APIKey  string
	Client  *transport.Client
}

// Provider adapts OpenAI-compatible APIs to the normalized llm.Provider API.
type Provider struct {
	name    string
	baseURL string
	apiKey  string
	client  *transport.Client
}

// New creates an OpenAI-compatible provider.
func New(config Config) *Provider {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		name = "openai"
	}
	client := config.Client
	if client == nil {
		client = transport.NewClient()
	}
	return &Provider{
		name:    name,
		baseURL: strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		apiKey:  strings.TrimSpace(config.APIKey),
		client:  client,
	}
}

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Capabilities() llm.Capability {
	return llm.Capability{Streaming: true}
}

func (p *Provider) validateMessages(messages []llm.Message) error {
	for _, message := range messages {
		if message.Role == llm.RoleTool {
			return fmt.Errorf("%s: tool messages are not supported by the M02 request model", p.name)
		}
	}
	return nil
}

// Chat sends a non-streaming chat completion request.
func (p *Provider) Chat(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	request.Stream = false
	response, err := p.doRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	result, err := llm.DecodeChatResponse(response.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.name, err)
	}
	return &result, nil
}

// ChatStream emits OpenAI SSE deltas until the provider sends [DONE].
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
			streamErr = fmt.Errorf("%s: stream ended before [DONE]", p.name)
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
		return nil, fmt.Errorf("openai provider is not initialized")
	}
	if p.baseURL == "" {
		return nil, fmt.Errorf("%s: base URL is required", p.name)
	}
	if err := p.validateMessages(request.Messages); err != nil {
		return nil, err
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("%s: encode request: %w", p.name, err)
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", p.name, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	response, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("%s: send request: %w", p.name, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		return nil, fmt.Errorf("%s: %s", p.name, apiError(response))
	}
	return response, nil
}

func parseDelta(data []byte) (delta string, done bool, err error) {
	if string(data) == "[DONE]" {
		return "", true, nil
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil {
		return "", false, fmt.Errorf("decode OpenAI stream event: %w", err)
	}
	if len(chunk.Choices) == 0 {
		return "", false, nil
	}
	return chunk.Choices[0].Delta.Content, false, nil
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

// NewOpenAI returns the public OpenAI API adapter.
func NewOpenAI(apiKey string) llm.Provider {
	return New(Config{Name: "openai", BaseURL: "https://api.openai.com/v1", APIKey: apiKey})
}

// NewDeepSeek returns DeepSeek's OpenAI-compatible API adapter.
func NewDeepSeek(apiKey string) llm.Provider {
	return New(Config{Name: "deepseek", BaseURL: "https://api.deepseek.com", APIKey: apiKey})
}

// NewDoubao returns ByteDance Ark's OpenAI-compatible API adapter.
func NewDoubao(apiKey string) llm.Provider {
	return New(Config{Name: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3", APIKey: apiKey})
}

// NewQwen returns Alibaba Cloud DashScope's OpenAI-compatible API adapter.
func NewQwen(apiKey string) llm.Provider {
	return New(Config{Name: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", APIKey: apiKey})
}

// NewOllama returns an adapter for Ollama's local OpenAI-compatible endpoint.
func NewOllama(baseURL string) llm.Provider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://localhost:11434/v1"
	}
	return New(Config{Name: "ollama", BaseURL: baseURL, APIKey: "ollama"})
}
