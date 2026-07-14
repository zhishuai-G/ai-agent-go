// minicall sends one question to an OpenAI-compatible chat-completions API.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/llm"
	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/transport"
)

// Config contains the provider values required for one non-streaming request.
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	question := strings.TrimSpace(strings.Join(os.Args[1:], " "))
	if question == "" {
		fmt.Fprintln(os.Stderr, "usage: minicall <question>")
		os.Exit(2)
	}

	config, err := loadConfigFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
	if err := runOnce(ctx, config, question); err != nil {
		fmt.Fprintln(os.Stderr, "request failed:", err)
		os.Exit(1)
	}
}

func loadConfigFromEnv() (Config, error) {
	config := Config{
		BaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_BASE_URL")), "/"),
		APIKey:  strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		Model:   strings.TrimSpace(os.Getenv("LLM_MODEL")),
	}
	return config, config.validate()
}

func (c Config) validate() error {
	switch {
	case c.BaseURL == "":
		return fmt.Errorf("LLM_BASE_URL is required")
	case c.APIKey == "":
		return fmt.Errorf("LLM_API_KEY is required")
	case c.Model == "":
		return fmt.Errorf("LLM_MODEL is required")
	}
	return nil
}

func runOnce(ctx context.Context, config Config, question string) error {
	return runOnceTo(ctx, config, question, os.Stdout, transport.NewClient())
}

func runOnceTo(ctx context.Context, config Config, question string, output io.Writer, client *transport.Client) error {
	if err := config.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(question) == "" {
		return fmt.Errorf("question is required")
	}
	if output == nil {
		return fmt.Errorf("output is nil")
	}
	if client == nil {
		return fmt.Errorf("client is nil")
	}

	payload, err := json.Marshal(llm.ChatRequest{
		Model: config.Model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: question},
		},
	})
	if err != nil {
		return fmt.Errorf("encode chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		config.BaseURL+"/chat/completions",
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("build chat request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return fmt.Errorf("provider returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	response, err := llm.DecodeChatResponse(resp.Body)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, response.Content); err != nil {
		return fmt.Errorf("write answer: %w", err)
	}
	if _, err := fmt.Fprintf(output, "token: input=%d output=%d\n", response.InputTokens, response.OutputTokens); err != nil {
		return fmt.Errorf("write token usage: %w", err)
	}

	return nil
}
