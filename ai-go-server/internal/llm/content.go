package llm

import (
	"encoding/json"
	"fmt"
)

// ContentPart is one part of a multimodal message.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL describes an image part in an OpenAI-compatible message.
type ImageURL struct {
	URL string `json:"url"`
}

// MessageContent accepts either a plain text string or an array of multimodal
// parts, which is how several OpenAI-compatible APIs represent content.
type MessageContent struct {
	Text  string
	Parts []ContentPart
}

func (m *MessageContent) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		m.Text = text
		m.Parts = nil
		return nil
	}

	var parts []ContentPart
	if err := json.Unmarshal(data, &parts); err == nil {
		m.Text = ""
		m.Parts = parts
		return nil
	}

	return fmt.Errorf("message content is neither a string nor an array: %s", data)
}

func (m MessageContent) MarshalJSON() ([]byte, error) {
	if len(m.Parts) > 0 {
		return json.Marshal(m.Parts)
	}
	return json.Marshal(m.Text)
}

// SafeJSON returns JSON for logs without risking a panic in an error path.
func SafeJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":"marshal failed: %v"}`, err)
	}
	return string(data)
}
