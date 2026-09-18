package modelcheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

func animationReadError(err error) error {
	// URL errors can embed userinfo or query credentials; retain only their cause.
	var endpoint *url.Error
	if errors.As(err, &endpoint) {
		err = endpoint.Err
	}
	label := "动画请求或事件流读取失败"
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &network) && network.Timeout()) {
		label = "动画检测请求超时，请调整请求超时或稍后重试"
	} else if errors.Is(err, context.Canceled) {
		label = "动画检测请求已取消"
	}
	return fmt.Errorf("%s：%w", label, err)
}

func animationVisibleError(err error, secrets ...string) error {
	message := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[已隐藏]")
		}
	}
	return visibleRequestError{message: safeCredentialText(message), cause: err}
}

func animationPayloadError(payload map[string]any, fallback string) error {
	if response, ok := payload["response"].(map[string]any); ok {
		payload = response
	}
	if nested, ok := payload["error"].(map[string]any); ok {
		payload = nested
	}
	parts := []string{}
	for _, key := range []string{"code", "message", "detail", "error"} {
		if value := stringField(payload, key); value != "" {
			parts = append(parts, value)
		}
	}
	if details, ok := payload["incomplete_details"].(map[string]any); ok {
		if reason := stringField(details, "reason"); reason != "" {
			parts = append(parts, reason)
		}
	}
	if len(parts) == 0 {
		return errors.New(fallback)
	}
	return fmt.Errorf("%s：%s", fallback, strings.Join(parts, "；"))
}
