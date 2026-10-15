package modelrequest

import (
	"strings"
	"unicode"
)

// Text extracts only model output. Reasoning, tools, metadata and usage never
// establish first text or appear in the user-visible probe reply.
func Text(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	str := func(v any) string { s, _ := v.(string); return s }
	if response, ok := object["response"].(map[string]any); ok {
		return Text(response)
	}
	if text := str(object["output_text"]); text != "" {
		return text
	}
	switch object["type"] {
	case "response.output_text.delta", "output_text.delta":
		return str(object["delta"])
	case "response.output_text.done", "output_text.done":
		return str(object["text"])
	case "content_block_delta":
		delta, _ := object["delta"].(map[string]any)
		if delta["type"] == "text_delta" {
			return str(delta["text"])
		}
		return ""
	case "content_block_start":
		block, _ := object["content_block"].(map[string]any)
		if block["type"] == "text" {
			return str(block["text"])
		}
		return ""
	}
	var out strings.Builder
	blocks := func(raw any) {
		items, _ := raw.([]any)
		for _, v := range items {
			block, _ := v.(map[string]any)
			if block["type"] == "text" || block["type"] == "output_text" {
				out.WriteString(str(block["text"]))
			}
		}
	}
	choices, _ := object["choices"].([]any)
	if len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		for _, key := range []string{"delta", "message"} {
			message, _ := choice[key].(map[string]any)
			out.WriteString(str(message["content"]))
			blocks(message["content"])
		}
	}
	blocks(object["content"])
	output, _ := object["output"].([]any)
	for _, v := range output {
		item, _ := v.(map[string]any)
		if item["type"] == "message" {
			blocks(item["content"])
		}
	}
	candidates, _ := object["candidates"].([]any)
	if len(candidates) > 0 {
		candidate, _ := candidates[0].(map[string]any)
		content, _ := candidate["content"].(map[string]any)
		parts, _ := content["parts"].([]any)
		for _, v := range parts {
			part, _ := v.(map[string]any)
			if part["thought"] != true {
				out.WriteString(str(part["text"]))
			}
		}
	}
	return out.String()
}

func MeaningfulText(text string) bool {
	if strings.ContainsFunc(text, func(r rune) bool {
		return unicode.Is(unicode.Han, r) || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || probeEmoji(r)
	}) {
		return true
	}
	return strings.Contains(text, "#\u20e3") || strings.Contains(text, "#\ufe0f\u20e3") || strings.Contains(text, "*\u20e3") || strings.Contains(text, "*\ufe0f\u20e3")
}

// TextAccumulator joins output deltas and ignores repeated terminal snapshots.
// Keeping the complete bounded response until redaction also catches secrets
// split across event boundaries; callers truncate only after redacting.
type TextAccumulator struct{ output strings.Builder }

func (a *TextAccumulator) Add(value any) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	text := Text(object)
	if text == "" {
		return
	}
	kind, _ := object["type"].(string)
	snapshot := kind == "response.completed" || kind == "response.done" || kind == "response.output_text.done" || kind == "output_text.done"
	if snapshot && a.output.Len() > 0 {
		return
	}
	a.output.WriteString(text)
}

func (a *TextAccumulator) String() string { return a.output.String() }
