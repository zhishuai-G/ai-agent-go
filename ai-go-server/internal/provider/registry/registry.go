// Package registry constructs the provider set from application configuration.
package registry

import (
	"strings"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/llm"
	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/provider/claude"
	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/provider/openai"
)

// Config contains the optional credentials and local endpoint used to register
// the providers available to a router.
type Config struct {
	OpenAIKey   string
	DeepSeekKey string
	DoubaoKey   string
	QwenKey     string
	ClaudeKey   string
	OllamaURL   string
}

// BuildAll returns only the providers for which credentials are configured,
// plus Ollama as a local development option.
func BuildAll(config Config) map[string]llm.Provider {
	providers := make(map[string]llm.Provider)
	if key := strings.TrimSpace(config.OpenAIKey); key != "" {
		providers["openai"] = openai.NewOpenAI(key)
	}
	if key := strings.TrimSpace(config.DeepSeekKey); key != "" {
		providers["deepseek"] = openai.NewDeepSeek(key)
	}
	if key := strings.TrimSpace(config.DoubaoKey); key != "" {
		providers["doubao"] = openai.NewDoubao(key)
	}
	if key := strings.TrimSpace(config.QwenKey); key != "" {
		providers["qwen"] = openai.NewQwen(key)
	}
	if key := strings.TrimSpace(config.ClaudeKey); key != "" {
		providers["claude"] = claude.New(key)
	}
	providers["ollama"] = openai.NewOllama(config.OllamaURL)
	return providers
}
