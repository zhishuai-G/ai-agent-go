package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeChatResponse(t *testing.T) {
	response, err := DecodeChatResponse(strings.NewReader(`{
		"choices":[{"message":{"role":"assistant","content":"hello"}}],
		"usage":{"prompt_tokens":7,"completion_tokens":11}
	}`))
	if err != nil {
		t.Fatalf("DecodeChatResponse() error = %v", err)
	}
	if response.Content != "hello" || response.InputTokens != 7 || response.OutputTokens != 11 {
		t.Fatalf("DecodeChatResponse() = %#v", response)
	}
}

func TestDecodeChatResponseNoChoices(t *testing.T) {
	_, err := DecodeChatResponse(strings.NewReader(`{"choices":[]}`))
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("DecodeChatResponse() error = %v, want no choices error", err)
	}
}

func TestMessageContentJSON(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantText   string
		wantParts  int
		wantOutput string
	}{
		{
			name:       "text",
			input:      `"hello"`,
			wantText:   "hello",
			wantOutput: `"hello"`,
		},
		{
			name:       "parts",
			input:      `[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]`,
			wantParts:  2,
			wantOutput: `[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var content MessageContent
			if err := json.Unmarshal([]byte(tt.input), &content); err != nil {
				t.Fatal(err)
			}
			if content.Text != tt.wantText || len(content.Parts) != tt.wantParts {
				t.Fatalf("decoded content = %#v", content)
			}
			encoded, err := json.Marshal(content)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(encoded); got != tt.wantOutput {
				t.Fatalf("MarshalJSON() = %s, want %s", got, tt.wantOutput)
			}
		})
	}
}

func TestParseInto(t *testing.T) {
	type weather struct {
		City string `json:"city"`
	}

	value, err := ParseInto[weather](`{"city":"Beijing"}`)
	if err != nil {
		t.Fatal(err)
	}
	if value.City != "Beijing" {
		t.Fatalf("City = %q", value.City)
	}
}

func TestNewChatRequestOptions(t *testing.T) {
	request := NewChatRequest(
		"test-model",
		[]Message{{Role: RoleUser, Content: "hello"}},
		WithTemperature(0),
		WithMaxTokens(128),
	)
	if request.Temperature == nil || *request.Temperature != 0 {
		t.Fatalf("Temperature = %v, want explicit zero", request.Temperature)
	}
	if request.MaxTokens != 128 {
		t.Fatalf("MaxTokens = %d", request.MaxTokens)
	}
}
