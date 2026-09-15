package newapimanagement

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strings"
)

type availableChannelPriceResponse struct {
	Data []availableChannelPrices `json:"data"`
}

type availableChannelPrices struct {
	Platforms       []availableChannelPricePlatform `json:"platforms"`
	SupportedModels []availableChannelPriceModel    `json:"supported_models"`
}

type availableChannelPricePlatform struct {
	SupportedModels []availableChannelPriceModel `json:"supported_models"`
}

type availableChannelPriceModel struct {
	Name    string                        `json:"name"`
	Pricing *availableChannelModelPricing `json:"pricing"`
}

type availableChannelModelPricing struct {
	BillingMode                  string            `json:"billing_mode"`
	InputPrice                   json.Number       `json:"input_price"`
	OutputPrice                  json.Number       `json:"output_price"`
	CacheWritePrice              json.Number       `json:"cache_write_price"`
	CacheWrite1hPrice            json.Number       `json:"cache_write_1h_price"`
	CacheReadPrice               json.Number       `json:"cache_read_price"`
	ImageInputPrice              json.Number       `json:"image_input_price"`
	ImageOutputPrice             json.Number       `json:"image_output_price"`
	PerRequestPrice              json.Number       `json:"per_request_price"`
	MaxReasoningEffortMultiplier json.Number       `json:"max_reasoning_effort_multiplier"`
	Intervals                    []json.RawMessage `json:"intervals"`
}

type comparableChannelPrice struct {
	price       ModelPrice
	fingerprint string
}

func decodeSub2APIAvailableChannelPricing(payload any) ([]ModelPrice, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("上游可用渠道价格响应无效，请检查上游接口后刷新")
	}
	var response availableChannelPriceResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return nil, errors.New("上游可用渠道价格结构或数值无效，请检查上游价卡后刷新")
	}
	byModel := make(map[string]comparableChannelPrice)
	excluded := make(map[string]bool)
	for _, channel := range response.Data {
		models := append([]availableChannelPriceModel(nil), channel.SupportedModels...)
		for _, platform := range channel.Platforms {
			models = append(models, platform.SupportedModels...)
		}
		for _, model := range models {
			name := strings.TrimSpace(model.Name)
			if len(normalizeModels([]string{name})) == 0 {
				continue
			}
			item, comparable, err := availableChannelComparablePrice(name, model.Pricing)
			if err != nil {
				return nil, err
			}
			previous, exists := byModel[name]
			if !comparable || (exists && previous.fingerprint != item.fingerprint) {
				excluded[name] = true
				delete(byModel, name)
				continue
			}
			if !excluded[name] {
				byModel[name] = item
			}
		}
	}
	prices := make([]ModelPrice, 0, len(byModel))
	for _, item := range byModel {
		prices = append(prices, item.price)
	}
	sort.Slice(prices, func(left, right int) bool { return prices[left].Model < prices[right].Model })
	if len(prices) == 0 {
		return nil, errors.New("上游可用渠道未返回可比价格，请确认已启用“可用渠道”、当前账号有可见渠道，且模型具有完整、无冲突的单价；阶梯及图片计费暂不支持比对")
	}
	return prices, nil
}

func availableChannelComparablePrice(name string, pricing *availableChannelModelPricing) (comparableChannelPrice, bool, error) {
	if pricing == nil {
		return comparableChannelPrice{}, false, nil
	}
	values := []json.Number{
		pricing.InputPrice, pricing.OutputPrice, pricing.CacheWritePrice, pricing.CacheWrite1hPrice,
		pricing.CacheReadPrice, pricing.ImageInputPrice, pricing.ImageOutputPrice, pricing.PerRequestPrice,
		pricing.MaxReasoningEffortMultiplier,
	}
	canonical := make([]string, len(values))
	for index, value := range values {
		if value == "" {
			continue
		}
		if !validDecimal(value.String()) {
			return comparableChannelPrice{}, false, errors.New("上游可用渠道包含无效价格，请检查模型单价是否为有效非负数后刷新")
		}
		number, _ := new(big.Rat).SetString(value.String())
		canonical[index] = number.RatString()
	}
	if len(pricing.Intervals) > 0 || pricing.ImageInputPrice != "" || pricing.ImageOutputPrice != "" {
		return comparableChannelPrice{}, false, nil
	}
	if pricing.MaxReasoningEffortMultiplier != "" && canonical[8] != "1" {
		return comparableChannelPrice{}, false, nil
	}
	mode := strings.TrimSpace(pricing.BillingMode)
	price := ModelPrice{Model: name}
	switch mode {
	case "", "token":
		mode = "token"
		input, output := pricing.InputPrice.String(), pricing.OutputPrice.String()
		inputRatio, completionRatio, ok := sub2APIRatios(input, output)
		if !ok || pricing.PerRequestPrice != "" {
			return comparableChannelPrice{}, false, nil
		}
		if inputRatio == "0" && (positiveChannelPrice(pricing.CacheWritePrice) || positiveChannelPrice(pricing.CacheWrite1hPrice) || positiveChannelPrice(pricing.CacheReadPrice)) {
			return comparableChannelPrice{}, false, nil
		}
		price.InputRatio, price.CompletionRatio = inputRatio, completionRatio
		price.InputPrice = multiplyDecimal(input, "1000000")
		price.CompletionPrice = multiplyDecimal(output, "1000000")
		price.CacheCreatePrice = multiplyDecimal(pricing.CacheWritePrice.String(), "1000000")
		price.CacheReadPrice = multiplyDecimal(pricing.CacheReadPrice.String(), "1000000")
		price.CacheRatio = priceRatio(input, pricing.CacheReadPrice.String())
		price.CreateCacheRatio = priceRatio(input, pricing.CacheWritePrice.String())
		price.CreateCache1hRatio = priceRatio(input, pricing.CacheWrite1hPrice.String())
	case "per_request":
		if pricing.PerRequestPrice == "" {
			return comparableChannelPrice{}, false, nil
		}
		price.ModelPrice = multiplyDecimal(pricing.PerRequestPrice.String(), "1")
		price.BillingMode = "per-request"
	default:
		return comparableChannelPrice{}, false, nil
	}
	return comparableChannelPrice{price: price, fingerprint: mode + ":" + strings.Join(canonical, ":")}, true, nil
}

func positiveChannelPrice(value json.Number) bool {
	if value == "" {
		return false
	}
	number, ok := new(big.Rat).SetString(value.String())
	return ok && number.Sign() > 0
}
