// Package prompt builds and budgets the messages sent to an LLM.
package prompt

import (
	"bytes"
	"fmt"
	"text/template"
)

// Template renders a text/template prompt with strict missing-key behavior.
type Template struct {
	template *template.Template
}

// New parses a prompt template. Missing variables cause an error at render
// time instead of silently producing <no value> in an LLM request.
func New(name, text string) (*Template, error) {
	parsed, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parse prompt template %q: %w", name, err)
	}
	return &Template{template: parsed}, nil
}

// Render executes the template with data.
func (t *Template) Render(data any) (string, error) {
	if t == nil || t.template == nil {
		return "", fmt.Errorf("prompt template is not initialized")
	}
	var buffer bytes.Buffer
	if err := t.template.Execute(&buffer, data); err != nil {
		return "", fmt.Errorf("render prompt template: %w", err)
	}
	return buffer.String(), nil
}
