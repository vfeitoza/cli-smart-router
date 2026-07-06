package infrastructure

import (
	"bytes"
	"encoding/json"
	"strings"
)

// ExtractUserPrompt pulls readable user text from an OpenAI/Claude/Gemini request
// body without forwarding the full original payload to a classifier or scorer.
// It returns the last user message when the body has recognizable message shapes,
// or the raw body text as a last resort so callers always have something to hash
// or score against.
func ExtractUserPrompt(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return ""
	}
	var parsed struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return string(body)
	}
	var out strings.Builder
	lastUser := ""
	for _, msg := range parsed.Messages {
		if msg.Role != "" && msg.Role != "user" {
			continue
		}
		if text := strings.TrimSpace(contentToText(msg.Content)); text != "" {
			lastUser = text
		}
	}
	if lastUser != "" {
		out.WriteString(lastUser)
		out.WriteString("\n")
	}
	for _, c := range parsed.Contents {
		for _, part := range c.Parts {
			out.WriteString(part.Text)
			out.WriteString("\n")
		}
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		return string(body)
	}
	return text
}

// contentToText flattens OpenAI/Claude string-or-array message content into plain text.
func contentToText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		return ""
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var out strings.Builder
		for _, p := range parts {
			if p.Text != "" {
				out.WriteString(p.Text)
				out.WriteString(" ")
			}
		}
		return strings.TrimSpace(out.String())
	}
	return ""
}
