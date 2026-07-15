package prompt

import (
	"fmt"
	"strings"

	"github.com/zhishuai-G/ai-agent-go/ai-go-server/internal/llm"
)

// Budget describes the maximum input-token allocation for one model call.
// Total should already reserve space for the model's completion output.
type Budget struct {
	Total        int
	SystemPrompt int
	Tools        int
	History      int
	Retrieved    int
}

// Usage records the estimated token use for the parts of one request.
// UserInput has no fixed sub-budget and consumes the remaining Total capacity.
type Usage struct {
	SystemPrompt int
	Tools        int
	History      int
	Retrieved    int
	UserInput    int
}

// TotalTokens returns the estimated token count across all context parts.
func (u Usage) TotalTokens() int {
	return u.SystemPrompt + u.Tools + u.History + u.Retrieved + u.UserInput
}

// Validate verifies that the configured sub-budgets can fit in Total.
func (b Budget) Validate() error {
	if b.Total <= 0 {
		return fmt.Errorf("total budget must be positive")
	}
	if b.SystemPrompt < 0 || b.Tools < 0 || b.History < 0 || b.Retrieved < 0 {
		return fmt.Errorf("budget values must not be negative")
	}
	if b.SystemPrompt+b.Tools+b.History+b.Retrieved > b.Total {
		return fmt.Errorf("allocated sub-budgets exceed total budget")
	}
	return nil
}

// Check returns an error when estimated usage exceeds a part's budget or the
// total input budget.
func (b Budget) Check(usage Usage) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if usage.SystemPrompt > b.SystemPrompt {
		return fmt.Errorf("system prompt uses %d tokens, budget is %d", usage.SystemPrompt, b.SystemPrompt)
	}
	if usage.Tools > b.Tools {
		return fmt.Errorf("tool definitions use %d tokens, budget is %d", usage.Tools, b.Tools)
	}
	if usage.History > b.History {
		return fmt.Errorf("history uses %d tokens, budget is %d", usage.History, b.History)
	}
	if usage.Retrieved > b.Retrieved {
		return fmt.Errorf("retrieved context uses %d tokens, budget is %d", usage.Retrieved, b.Retrieved)
	}
	if usage.TotalTokens() > b.Total {
		return fmt.Errorf("context uses %d tokens, total budget is %d", usage.TotalTokens(), b.Total)
	}
	return nil
}

// EstimateTokens provides a deliberately rough token estimate for budget
// guardrails: English is approximately four characters per token and CJK text
// is approximately 1.5 to two characters per token. Provider usage remains
// authoritative for billing and exact limits.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	var ascii, nonASCII int
	for _, runeValue := range text {
		if runeValue < 128 {
			ascii++
		} else {
			nonASCII++
		}
	}
	return ascii/4 + nonASCII*2/3 + 1
}

// EstimateContext calculates a budget report without mutating any input.
func EstimateContext(system string, tools []string, turns []Turn, retrieved []string, userInput string) Usage {
	usage := Usage{
		SystemPrompt: EstimateTokens(system),
		UserInput:    EstimateTokens(userInput),
	}
	for _, definition := range tools {
		usage.Tools += EstimateTokens(definition)
	}
	for _, turn := range turns {
		usage.History += turn.EstimateTokens()
	}
	for _, document := range retrieved {
		usage.Retrieved += EstimateTokens(document)
	}
	return usage
}

// Turn is one complete user/assistant exchange. Keeping complete turns avoids
// orphaned assistant messages when history is trimmed.
type Turn struct {
	User      string
	Assistant string
}

// EstimateTokens returns the estimated cost of both messages in a turn.
func (t Turn) EstimateTokens() int {
	return EstimateTokens(t.User) + EstimateTokens(t.Assistant)
}

// TrimTurns retains the newest complete turns that fit within tokenLimit.
// The returned slice does not share its backing array with turns.
func TrimTurns(turns []Turn, tokenLimit int) []Turn {
	if tokenLimit <= 0 || len(turns) == 0 {
		return nil
	}
	used := 0
	start := len(turns)
	for index := len(turns) - 1; index >= 0; index-- {
		cost := turns[index].EstimateTokens()
		if used+cost > tokenLimit {
			break
		}
		used += cost
		start = index
	}
	if start == len(turns) {
		return nil
	}
	return append([]Turn(nil), turns[start:]...)
}

// BuildMessages assembles the provider-neutral message list in cache-friendly
// order: stable system instructions first, bounded historical turns next, and
// the current user input last.
func BuildMessages(system string, turns []Turn, userInput string, historyBudget int) ([]llm.Message, error) {
	if strings.TrimSpace(userInput) == "" {
		return nil, fmt.Errorf("user input is required")
	}

	messages := make([]llm.Message, 0, 1+len(turns)*2+1)
	if strings.TrimSpace(system) != "" {
		messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: system})
	}
	for _, turn := range TrimTurns(turns, historyBudget) {
		messages = append(messages,
			llm.Message{Role: llm.RoleUser, Content: turn.User},
			llm.Message{Role: llm.RoleAssistant, Content: turn.Assistant},
		)
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userInput})
	return messages, nil
}
