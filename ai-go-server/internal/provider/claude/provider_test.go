package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/llm"
)

func TestAdaptRequest(t *testing.T) {
	provider := New("test-key")
	body, err := provider.adaptRequest(llm.NewChatRequest(
		"claude-test",
		[]llm.Message{
			{Role: llm.RoleSystem, Content: "be concise"},
			{Role: llm.RoleUser, Content: "hello"},
		},
		llm.WithTemperature(0.2),
	))
	if err != nil {
		t.Fatal(err)
	}
	if body["system"] != "be concise" || body["max_tokens"] != 1024 || body["temperature"] != 0.2 {
		t.Fatalf("body = %#v", body)
	}
	messages := body["messages"].([]map[string]string)
	if len(messages) != 1 || messages[0]["role"] != "user" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestProviderChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Fatalf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Fatalf("anthropic-version = %q", got)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["system"] != "be concise" || request["max_tokens"] != float64(1024) {
			t.Fatalf("request = %#v", request)
		}
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Hello"},{"type":"tool_use"},{"type":"text","text":"!"}],"usage":{"input_tokens":2,"output_tokens":3}}`))
	}))
	defer server.Close()

	provider := NewWithConfig(Config{BaseURL: server.URL, APIKey: "test-key"})
	response, err := provider.Chat(context.Background(), llm.NewChatRequest("claude-test", []llm.Message{
		{Role: llm.RoleSystem, Content: "be concise"},
		{Role: llm.RoleUser, Content: "hello"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "Hello!" || response.InputTokens != 2 || response.OutputTokens != 3 {
		t.Fatalf("response = %#v", response)
	}
}

func TestProviderChatStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider := NewWithConfig(Config{BaseURL: server.URL, APIKey: "test-key"})
	chunks, err := provider.ChatStream(context.Background(), llm.NewChatRequest("claude-test", []llm.Message{{Role: llm.RoleUser, Content: "hello"}}))
	if err != nil {
		t.Fatal(err)
	}
	var content strings.Builder
	for chunk := range chunks {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
		content.WriteString(chunk.Content)
	}
	if got := content.String(); got != "Hi" {
		t.Fatalf("stream content = %q", got)
	}
}
