package modelcheck

import (
	"errors"
	"strings"
)

// Extract one document for validation while retaining the original reply in Source.
func extractAnimationDocument(source string) (string, error) {
	source = strings.TrimSpace(source)
	if animationDocumentStart(source) {
		return source, nil
	}
	var marker byte
	var width int
	var body strings.Builder
	var candidate string
	for _, line := range strings.SplitAfter(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if marker == 0 {
			if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
				continue
			}
			count := fenceWidth(trimmed, trimmed[0])
			if count < 3 {
				continue
			}
			marker, width = trimmed[0], count
			body.Reset()
			continue
		}
		count := fenceWidth(trimmed, marker)
		if count >= width && strings.TrimSpace(trimmed[count:]) == "" {
			document := strings.TrimSpace(body.String())
			if animationDocumentStart(document) {
				if candidate != "" {
					return "", errors.New("返回内容包含多个动画代码块，请让模型只返回一个完整 HTML 或 SVG")
				}
				candidate = document
			}
			marker = 0
			continue
		}
		body.WriteString(line)
	}
	if marker != 0 && animationDocumentStart(strings.TrimSpace(body.String())) {
		return "", errors.New("动画代码块未完整结束，请重试")
	}
	if candidate != "" {
		return candidate, nil
	}
	return "", errors.New("返回内容未包含完整的 HTML 或 SVG 动画代码块，请重试")
}

func fenceWidth(line string, marker byte) int {
	count := 0
	for count < len(line) && line[count] == marker {
		count++
	}
	return count
}

func animationDocumentStart(source string) bool {
	lower := strings.ToLower(source)
	for _, prefix := range []string{"<!doctype html", "<html", "<svg"} {
		if strings.HasPrefix(lower, prefix) && len(lower) > len(prefix) && strings.ContainsRune(" >\t\r\n", rune(lower[len(prefix)])) {
			return true
		}
	}
	return false
}
