package registry

import "testing"

func TestBuildAll(t *testing.T) {
	providers := BuildAll(Config{DeepSeekKey: "deepseek-key", ClaudeKey: "claude-key"})
	if providers["deepseek"].Name() != "deepseek" {
		t.Fatalf("deepseek provider = %#v", providers["deepseek"])
	}
	if providers["claude"].Name() != "claude" {
		t.Fatalf("claude provider = %#v", providers["claude"])
	}
	if providers["ollama"].Name() != "ollama" {
		t.Fatalf("ollama provider = %#v", providers["ollama"])
	}
	if _, exists := providers["openai"]; exists {
		t.Fatal("openai was registered without a key")
	}
}
