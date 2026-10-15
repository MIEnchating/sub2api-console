package modelrequest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
)

// observedBody observes SSE without changing bytes or owning domain success
// decisions. Protocol readers still require their own valid completion event.
type observedBody struct {
	io.ReadCloser
	ctx         context.Context
	closeBudget func()
	observe     bool
	line        []byte
	data        []byte
	event       string
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if b.observe {
		for _, c := range p[:n] {
			if c != '\n' {
				if len(b.line) >= 1<<20 {
					b.observe = false
					break
				}
				b.line = append(b.line, c)
				continue
			}
			b.consumeLine()
			b.line = b.line[:0]
			if !b.observe {
				break
			}
		}
	}
	if err != nil && b.ctx.Err() != nil {
		if context.Cause(b.ctx) == context.DeadlineExceeded {
			return n, context.DeadlineExceeded
		}
		return n, b.ctx.Err()
	}
	return n, err
}

func (b *observedBody) Close() error { err := b.ReadCloser.Close(); b.closeBudget(); return err }

func (b *observedBody) consumeLine() {
	line := bytes.TrimSuffix(b.line, []byte{'\r'})
	if bytes.HasPrefix(bytes.TrimSpace(line), []byte("{")) {
		b.observe = false
		return
	} // JSON fallback is non-streaming.
	if bytes.HasPrefix(line, []byte("event:")) {
		b.event = strings.TrimSpace(string(line[6:]))
		return
	}
	if len(line) == 0 {
		b.data = nil
		b.event = ""
		return
	}
	if !bytes.HasPrefix(line, []byte("data:")) {
		return
	}
	part := bytes.TrimSpace(line[5:])
	if len(b.data) > 0 {
		b.data = append(b.data, '\n')
	}
	if len(b.data)+len(part) > 1<<20 {
		b.observe = false
		return
	}
	b.data = append(b.data, part...)
	var event map[string]any
	if json.Unmarshal(b.data, &event) != nil {
		return
	}
	if event["type"] == nil && b.event != "" {
		event["type"] = b.event
	}
	if len(event) > 0 {
		receivedEvent(b.ctx)
	}
	b.data = nil
}
