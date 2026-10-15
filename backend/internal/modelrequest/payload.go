package modelrequest

import (
	"errors"
	"strings"
)

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type reasoning struct {
	Effort string `json:"effort"`
}
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}
type responsesRequest struct {
	Model           string     `json:"model"`
	Input           any        `json:"input"`
	MaxOutputTokens int        `json:"max_output_tokens,omitempty"`
	Stream          bool       `json:"stream"`
	Reasoning       *reasoning `json:"reasoning,omitempty"`
	Instructions    string     `json:"instructions,omitempty"`
	Store           *bool      `json:"store,omitempty"`
}
type chatRequest struct {
	Model           string         `json:"model"`
	Messages        []message      `json:"messages"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	Stream          bool           `json:"stream"`
	ReasoningEffort string         `json:"reasoning_effort,omitempty"`
	StreamOptions   *streamOptions `json:"stream_options,omitempty"`
}
type anthropicRequest struct {
	Model        string     `json:"model"`
	Messages     []message  `json:"messages"`
	MaxTokens    int        `json:"max_tokens,omitempty"`
	Stream       bool       `json:"stream"`
	OutputConfig *reasoning `json:"output_config,omitempty"`
}
type geminiPart struct {
	Text string `json:"text"`
}
type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}
type geminiConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens"`
}
type geminiRequest struct {
	Contents         []geminiContent `json:"contents"`
	GenerationConfig *geminiConfig   `json:"generationConfig,omitempty"`
}

// Build is also used to fingerprint the exact request before sending it.
// Protocol-specific wire fields belong here, never in a business feature.
func Build(protocol Protocol, p Parameters) (string, any, error) {
	var effort *reasoning
	if p.ReasoningEffort != "" {
		effort = &reasoning{Effort: p.ReasoningEffort}
	}
	messages := []message{{Role: "user", Content: p.Prompt}}
	switch protocol {
	case Responses:
		var input any = p.Prompt
		if p.InputMessages {
			input = messages
		}
		return "/v1/responses", responsesRequest{Model: p.Model, Input: input, MaxOutputTokens: p.MaxOutputTokens, Stream: p.Stream, Reasoning: effort, Instructions: p.Instructions, Store: p.Store}, nil
	case Chat:
		var options *streamOptions
		if p.Stream && p.IncludeUsage {
			options = &streamOptions{IncludeUsage: true}
		}
		return "/v1/chat/completions", chatRequest{Model: p.Model, Messages: messages, MaxTokens: p.MaxOutputTokens, Stream: p.Stream, ReasoningEffort: p.ReasoningEffort, StreamOptions: options}, nil
	case Anthropic:
		return "/v1/messages", anthropicRequest{Model: p.Model, Messages: messages, MaxTokens: p.MaxOutputTokens, Stream: p.Stream, OutputConfig: effort}, nil
	case Gemini:
		model := strings.TrimPrefix(strings.TrimSpace(p.Model), "models/")
		if model == "" || strings.ContainsAny(model, "/\\?#%") || model == "." || model == ".." {
			return "", nil, errors.New("Gemini 模型 ID 无效")
		}
		path := "/v1beta/models/" + model + ":generateContent"
		if p.Stream {
			path = "/v1beta/models/" + model + ":streamGenerateContent?alt=sse"
		}
		var config *geminiConfig
		if p.MaxOutputTokens > 0 {
			config = &geminiConfig{MaxOutputTokens: p.MaxOutputTokens}
		}
		return path, geminiRequest{Contents: []geminiContent{{Role: "user", Parts: []geminiPart{{Text: p.Prompt}}}}, GenerationConfig: config}, nil
	default:
		return "", nil, errors.New("不支持的模型请求协议")
	}
}
