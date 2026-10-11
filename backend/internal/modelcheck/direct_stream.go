package modelcheck

import (
	"io"
	"strings"
)

func (sender directBundleSender) readStream(reader io.Reader, path string, status int) (map[string]any, int, []byte, error) {
	text, model, err := animationStream(reader, path, sender.usage)
	if err != nil {
		message := strings.ReplaceAll(err.Error(), "动画", "前置检测")
		if sender.credential.Secret != "" {
			message = strings.ReplaceAll(message, sender.credential.Secret, "[已隐藏]")
		}
		return nil, status, nil, visibleRequestError{message: safeCredentialText(message)}
	}
	if sender.credential.Secret != "" {
		model = strings.ReplaceAll(model, sender.credential.Secret, "[已隐藏]")
	}
	model = safeCredentialText(model)
	// Adapt the shared stream reader to the existing protocol-specific text readers.
	return map[string]any{
		"model": model, "output_text": text,
		"content": []any{map[string]any{"type": "text", "text": text}},
		"choices": []any{map[string]any{"message": map[string]any{"content": text}}},
	}, status, nil, nil
}
