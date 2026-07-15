// docqa answers a question using only local document files supplied at runtime.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/llm"
	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/prompt"
	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/provider/openai"
)

type config struct {
	baseURL string
	apiKey  string
	model   string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "docqa failed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("docqa", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	product := flags.String("product", "产品", "产品名称")
	documentPaths := flags.String("docs", "", "逗号分隔的本地资料文件路径")
	inputBudget := flags.Int("input-budget", 8000, "可用输入 token 预算（已预留输出空间）")
	maxTokens := flags.Int("max-tokens", 1024, "模型最大输出 token")
	if err := flags.Parse(args); err != nil {
		return err
	}
	question := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if question == "" {
		return fmt.Errorf("usage: docqa -docs file1,file2 [-product name] <question>")
	}

	configuration, err := loadConfigFromEnv()
	if err != nil {
		return err
	}
	documents, err := readDocuments(*documentPaths)
	if err != nil {
		return err
	}
	request, usage, err := buildRequest(configuration.model, *product, documents, question, prompt.Budget{
		Total:        *inputBudget,
		SystemPrompt: *inputBudget,
	}, *maxTokens)
	if err != nil {
		return err
	}

	provider := openai.New(openai.Config{
		Name:    "openai-compatible",
		BaseURL: configuration.baseURL,
		APIKey:  configuration.apiKey,
	})
	response, err := provider.Chat(ctx, request)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, response.Content); err != nil {
		return fmt.Errorf("write answer: %w", err)
	}
	if _, err := fmt.Fprintf(output, "token: input=%d output=%d (estimated-context=%d)\n", response.InputTokens, response.OutputTokens, usage.TotalTokens()); err != nil {
		return fmt.Errorf("write token usage: %w", err)
	}
	return nil
}

func loadConfigFromEnv() (config, error) {
	configuration := config{
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_BASE_URL")), "/"),
		apiKey:  strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		model:   strings.TrimSpace(os.Getenv("LLM_MODEL")),
	}
	switch {
	case configuration.baseURL == "":
		return config{}, fmt.Errorf("LLM_BASE_URL is required")
	case configuration.apiKey == "":
		return config{}, fmt.Errorf("LLM_API_KEY is required")
	case configuration.model == "":
		return config{}, fmt.Errorf("LLM_MODEL is required")
	}
	return configuration, nil
}

func readDocuments(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("-docs is required")
	}

	paths := strings.Split(value, ",")
	documents := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read document %q: %w", path, err)
		}
		documents = append(documents, fmt.Sprintf("来源：%s\n%s", filepath.Base(path), strings.TrimSpace(string(content))))
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("-docs contains no usable document paths")
	}
	return documents, nil
}

func buildRequest(model, product string, documents []string, question string, budget prompt.Budget, maxTokens int) (llm.ChatRequest, prompt.Usage, error) {
	if maxTokens <= 0 {
		return llm.ChatRequest{}, prompt.Usage{}, fmt.Errorf("max tokens must be positive")
	}
	system, err := prompt.RenderDocumentAssistant(prompt.DocumentAssistantData{
		Product: product,
		Docs:    documents,
	})
	if err != nil {
		return llm.ChatRequest{}, prompt.Usage{}, err
	}
	usage := prompt.EstimateContext(system, nil, nil, nil, question)
	if err := budget.Check(usage); err != nil {
		return llm.ChatRequest{}, usage, err
	}
	messages, err := prompt.BuildMessages(system, nil, question, 0)
	if err != nil {
		return llm.ChatRequest{}, usage, err
	}
	return llm.NewChatRequest(model, messages, llm.WithMaxTokens(maxTokens)), usage, nil
}
