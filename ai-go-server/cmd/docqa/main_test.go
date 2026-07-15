package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/llm"
	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/prompt"
)

func TestBuildRequest(t *testing.T) {
	request, usage, err := buildRequest(
		"test-model",
		"示例产品",
		[]string{"来源：guide.md\ntimeout 默认 30 秒"},
		"默认超时是多少？",
		prompt.Budget{Total: 8000, SystemPrompt: 8000},
		512,
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Model != "test-model" || request.MaxTokens != 512 || len(request.Messages) != 2 {
		t.Fatalf("request = %#v", request)
	}
	if request.Messages[0].Role != llm.RoleSystem || !strings.Contains(request.Messages[0].Content, "timeout 默认 30 秒") {
		t.Fatalf("system message = %#v", request.Messages[0])
	}
	if request.Messages[1].Role != llm.RoleUser || request.Messages[1].Content != "默认超时是多少？" {
		t.Fatalf("user message = %#v", request.Messages[1])
	}
	if usage.TotalTokens() == 0 {
		t.Fatal("usage was not estimated")
	}
}

func TestBuildRequestRejectsOverBudget(t *testing.T) {
	_, _, err := buildRequest(
		"test-model",
		"产品",
		[]string{strings.Repeat("资料", 100)},
		"问题",
		prompt.Budget{Total: 10, SystemPrompt: 10},
		128,
	)
	if err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("buildRequest() error = %v", err)
	}
}

func TestReadDocuments(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "guide.md")
	if err := os.WriteFile(path, []byte("配置 timeout"), 0o600); err != nil {
		t.Fatal(err)
	}
	documents, err := readDocuments(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(documents) != 1 || !strings.Contains(documents[0], "guide.md") || !strings.Contains(documents[0], "配置 timeout") {
		t.Fatalf("documents = %#v", documents)
	}
}
