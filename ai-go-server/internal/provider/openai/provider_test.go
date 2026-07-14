package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/llm"
)

func TestProviderChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		var request llm.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "test-model" || len(request.Messages) != 1 || request.Stream {
			t.Fatalf("request = %#v", request)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":4,"completion_tokens":6}}`))
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "test-key"})
	response, err := provider.Chat(context.Background(), llm.NewChatRequest("test-model", []llm.Message{{Role: llm.RoleUser, Content: "hi"}}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "hello" || response.InputTokens != 4 || response.OutputTokens != 6 {
		t.Fatalf("response = %#v", response)
	}
}

func TestProviderChatRejectsToolMessage(t *testing.T) {
	provider := New(Config{Name: "test", BaseURL: "https://example.com", APIKey: "key"})
	_, err := provider.Chat(context.Background(), llm.NewChatRequest("test", []llm.Message{{Role: llm.RoleTool, Content: "result"}}))
	if err == nil || !strings.Contains(err.Error(), "tool messages") {
		t.Fatalf("Chat() error = %v", err)
	}
}

func TestProviderChatStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := New(Config{BaseURL: server.URL})
	chunks, err := provider.ChatStream(context.Background(), llm.NewChatRequest("test", []llm.Message{{Role: llm.RoleUser, Content: "hi"}}))
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
	if got := content.String(); got != "Hello" {
		t.Fatalf("stream content = %q", got)
	}
}

func TestParseDelta(t *testing.T) {
	_, done, err := parseDelta([]byte("[DONE]"))
	if err != nil || !done {
		t.Fatalf("parseDelta([DONE]) = done:%v err:%v", done, err)
	}
}
