// Package router selects an LLM provider without exposing protocol details to
// Agent code.
package router

import (
	"context"
	"fmt"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/llm"
)

// Strategy decides the provider attempt order for one request.
type Strategy interface {
	Order([]llm.Provider) []llm.Provider
}

// Priority tries providers in registration order and therefore supports a
// simple primary-provider then fallback-provider policy.
type Priority struct{}

func (Priority) Order(providers []llm.Provider) []llm.Provider {
	return append([]llm.Provider(nil), providers...)
}

// Router applies a strategy to a fixed set of providers.
type Router struct {
	providers []llm.Provider
	strategy  Strategy
}

// New validates and copies the provider set.
func New(strategy Strategy, providers ...llm.Provider) (*Router, error) {
	if strategy == nil {
		return nil, fmt.Errorf("router strategy is required")
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("at least one provider is required")
	}
	for index, provider := range providers {
		if provider == nil {
			return nil, fmt.Errorf("providers[%d] is nil", index)
		}
	}
	return &Router{
		providers: append([]llm.Provider(nil), providers...),
		strategy:  strategy,
	}, nil
}

// Name makes Router usable anywhere an llm.Provider is expected.
func (r *Router) Name() string {
	return "router"
}

// Capabilities reports the capabilities available from at least one route.
func (r *Router) Capabilities() llm.Capability {
	var capability llm.Capability
	if r == nil {
		return capability
	}
	for _, provider := range r.providers {
		providerCapability := provider.Capabilities()
		capability.Streaming = capability.Streaming || providerCapability.Streaming
		capability.Thinking = capability.Thinking || providerCapability.Thinking
		capability.Tools = capability.Tools || providerCapability.Tools
	}
	return capability
}

// Chat routes a non-streaming request and discards the successful provider name.
func (r *Router) Chat(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	response, _, err := r.RouteChat(ctx, request)
	return response, err
}

// RouteChat returns the successful provider name for metrics and cost tracking.
func (r *Router) RouteChat(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, string, error) {
	if r == nil {
		return nil, "", fmt.Errorf("router is nil")
	}
	ordered := r.strategy.Order(r.providers)
	var lastErr error
	for _, provider := range ordered {
		if provider == nil {
			continue
		}
		response, err := provider.Chat(ctx, request)
		if err == nil {
			return response, provider.Name(), nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
	}
	if lastErr == nil {
		return nil, "", fmt.Errorf("router strategy returned no providers")
	}
	return nil, "", fmt.Errorf("all providers failed: %w", lastErr)
}

// ChatStream selects the first streaming-capable provider that accepts the
// request. A provider cannot be swapped after it has begun emitting a stream.
func (r *Router) ChatStream(ctx context.Context, request llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if r == nil {
		return nil, fmt.Errorf("router is nil")
	}
	var lastErr error
	for _, provider := range r.strategy.Order(r.providers) {
		if provider == nil || !provider.Capabilities().Streaming {
			continue
		}
		chunks, err := provider.ChatStream(ctx, request)
		if err == nil {
			return chunks, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if lastErr == nil {
		return nil, fmt.Errorf("no streaming-capable provider is available")
	}
	return nil, fmt.Errorf("all streaming providers failed: %w", lastErr)
}
