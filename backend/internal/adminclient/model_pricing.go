package adminclient

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

// DefaultModelPricing contains USD per-token values from Sub2API's billing service.
type DefaultModelPricing struct {
	Found             bool
	InputPrice        string
	OutputPrice       string
	CacheWritePrice   string
	CacheWrite1hPrice string
	CacheReadPrice    string
	ImageInputPrice   string
	ImageOutputPrice  string
}

func (c *Client) ModelPricing(ctx context.Context, model string) (DefaultModelPricing, error) {
	if strings.TrimSpace(model) == "" || utf8.RuneCountInString(model) > 256 {
		return DefaultModelPricing{}, errors.New("模型名称无效")
	}
	payload, err := c.request(ctx, http.MethodGet, "/admin/channels/model-pricing", nil, map[string]string{"model": model})
	if err != nil {
		return DefaultModelPricing{}, err
	}
	data, err := responseObject(payload, "模型默认价格")
	if err != nil {
		return DefaultModelPricing{}, err
	}
	found, ok := data["found"].(bool)
	if !ok {
		return DefaultModelPricing{}, errors.New("模型默认价格缺少 found 状态")
	}
	result := DefaultModelPricing{Found: found}
	if !found {
		return result, nil
	}
	fields := []struct {
		key      string
		target   *string
		required bool
	}{
		{"input_price", &result.InputPrice, true}, {"output_price", &result.OutputPrice, true},
		{"cache_write_price", &result.CacheWritePrice, false}, {"cache_write_1h_price", &result.CacheWrite1hPrice, false},
		{"cache_read_price", &result.CacheReadPrice, false}, {"image_input_price", &result.ImageInputPrice, false}, {"image_output_price", &result.ImageOutputPrice, false},
	}
	for _, field := range fields {
		value := data[field.key]
		if value == nil && !field.required {
			continue
		}
		decimal, err := exactJSONDecimal(value)
		if err != nil || strings.HasPrefix(decimal, "-") {
			return DefaultModelPricing{}, errors.New("模型默认价格包含无效 " + field.key)
		}
		*field.target = decimal
	}
	return result, nil
}
