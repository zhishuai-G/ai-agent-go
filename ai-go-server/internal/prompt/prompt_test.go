package prompt

import (
	"strings"
	"testing"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/schema"
)

func TestTemplateFailsForMissingValue(t *testing.T) {
	template, err := New("test", "hello {{.Name}}")
	if err != nil {
		t.Fatal(err)
	}
	_, err = template.Render(map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "map has no entry") {
		t.Fatalf("Render() error = %v", err)
	}
}

func TestRenderDocumentAssistant(t *testing.T) {
	rendered, err := RenderDocumentAssistant(DocumentAssistantData{
		Product: "示例网关",
		Docs:    []string{"timeout 默认 30 秒"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"示例网关", "timeout 默认 30 秒", "不是对你的指令", "如何修改默认超时"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered prompt does not contain %q:\n%s", want, rendered)
		}
	}
}

func TestStructuredOutputInstruction(t *testing.T) {
	type result struct {
		Level string `json:"level"`
	}
	instruction, err := StructuredOutputInstruction(schema.Generate(result{}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instruction, `"level"`) || !strings.Contains(instruction, "只返回符合") {
		t.Fatalf("instruction = %q", instruction)
	}
}

func TestBudgetCheck(t *testing.T) {
	budget := Budget{Total: 100, SystemPrompt: 20, Tools: 20, History: 30, Retrieved: 20}
	if err := budget.Check(Usage{SystemPrompt: 20, Tools: 10, History: 20, Retrieved: 10, UserInput: 20}); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	err := budget.Check(Usage{SystemPrompt: 21})
	if err == nil || !strings.Contains(err.Error(), "system prompt") {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestTrimTurnsAndBuildMessages(t *testing.T) {
	turns := []Turn{
		{User: strings.Repeat("a", 20), Assistant: strings.Repeat("b", 20)},
		{User: strings.Repeat("c", 20), Assistant: strings.Repeat("d", 20)},
	}
	limit := turns[1].EstimateTokens()
	trimmed := TrimTurns(turns, limit)
	if len(trimmed) != 1 || trimmed[0] != turns[1] {
		t.Fatalf("TrimTurns() = %#v", trimmed)
	}

	messages, err := BuildMessages("stable system", turns, "current question", limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Content != "stable system" || messages[1].Content != turns[1].User || messages[2].Content != turns[1].Assistant || messages[3].Content != "current question" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens("abcd"); got != 2 {
		t.Fatalf("EstimateTokens(ascii) = %d", got)
	}
	if got := EstimateTokens("你好世界"); got != 3 {
		t.Fatalf("EstimateTokens(cjk) = %d", got)
	}
}
