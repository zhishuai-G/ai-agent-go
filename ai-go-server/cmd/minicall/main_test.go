package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/provider/openai"
)

func TestRunOnceTo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}

		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "test-model" || len(request.Messages) != 1 || request.Messages[0].Content != "say hello" {
			t.Fatalf("payload = %#v", request)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":3,"completion_tokens":5}}`))
	}))
	defer server.Close()

	var output bytes.Buffer
	err := runOnceTo(context.Background(), Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "test-model",
	}, "say hello", &output, openai.New(openai.Config{
		Name:    "test",
		BaseURL: server.URL,
		APIKey:  "test-key",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "hello\ntoken: input=3 output=5\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestConfigValidate(t *testing.T) {
	if err := (Config{}).validate(); err == nil {
		t.Fatal("validate() unexpectedly succeeded")
	}
}
