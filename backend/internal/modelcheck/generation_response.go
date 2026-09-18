package modelcheck

import (
	"bufio"
	"errors"
)

// Inspect only the leading bytes, so SSE parsing can start before the body ends.
// Some compatible endpoints return JSON even when streaming was requested.
func generationResponseIsJSON(reader *bufio.Reader) (bool, error) {
	for skipped := 0; skipped < maximumDirectResponseBytes; skipped++ {
		prefix, err := reader.Peek(1)
		if err != nil {
			return false, err
		}
		switch prefix[0] {
		case ' ', '\t', '\n', '\r':
			_, _ = reader.Discard(1)
		case 0xef:
			bom, err := reader.Peek(3)
			if err != nil || string(bom) != "\xef\xbb\xbf" {
				return false, errors.New("上游响应编码无效")
			}
			_, _ = reader.Discard(3)
		default:
			return prefix[0] == '{', nil
		}
	}
	return false, errors.New("上游响应过大")
}
