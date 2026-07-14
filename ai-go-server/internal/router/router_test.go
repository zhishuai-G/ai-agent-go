package router

import (
	"context"
	"errors"
	"testing"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/llm"
)

type fakeProvider struct {
	name       string
	capability llm.Capability
	chat       func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error)
	stream     func(context.Context, llm.ChatRequest) (<-chan llm.StreamChunk, error)
}

func (p fakeProvider) Name() string {
	return p.name
}

func (p fakeProvider) Capabilities() llm.Capability {
	return p.capability
}

func (p fakeProvider) Chat(ctx context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	return p.chat(ctx, request)
}

func (p fakeProvider) ChatStream(ctx context.Context, request llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if p.stream == nil {
		return nil, errors.New("stream unavailable")
	}
	return p.stream(ctx, request)
}

func TestRouteChatFallsBack(t *testing.T) {
	primary := fakeProvider{
		name: "primary",
		chat: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, errors.New("primary unavailable")
		},
	}
	fallback := fakeProvider{
		name: "fallback",
		chat: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{Content: "answer"}, nil
		},
	}
	router, err := New(Priority{}, primary, fallback)
	if err != nil {
		t.Fatal(err)
	}

	response, provider, err := router.RouteChat(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if provider != "fallback" || response.Content != "answer" {
		t.Fatalf("response = %#v provider = %q", response, provider)
	}
}

func TestRouteChatStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calledFallback := false
	primary := fakeProvider{
		name: "primary",
		chat: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, errors.New("failed")
		},
	}
	fallback := fakeProvider{
		name: "fallback",
		chat: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
			calledFallback = true
			return nil, errors.New("should not run")
		},
	}
	router, err := New(Priority{}, primary, fallback)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = router.RouteChat(ctx, llm.ChatRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RouteChat() error = %v", err)
	}
	if calledFallback {
		t.Fatal("fallback was called after cancellation")
	}
}

func TestRouterChatStreamSkipsNonStreamingProvider(t *testing.T) {
	nonStreaming := fakeProvider{
		name: "non-streaming",
		chat: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) { return nil, nil },
	}
	streaming := fakeProvider{
		name:       "streaming",
		capability: llm.Capability{Streaming: true},
		chat:       func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) { return nil, nil },
		stream: func(context.Context, llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			chunks := make(chan llm.StreamChunk, 1)
			chunks <- llm.StreamChunk{Content: "ok"}
			close(chunks)
			return chunks, nil
		},
	}
	router, err := New(Priority{}, nonStreaming, streaming)
	if err != nil {
		t.Fatal(err)
	}

	chunks, err := router.ChatStream(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	chunk := <-chunks
	if chunk.Content != "ok" {
		t.Fatalf("chunk = %#v", chunk)
	}
}
