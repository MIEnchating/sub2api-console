package officialpricing

import (
	"errors"
	"strings"
)

// Image and text token prices are distinct. Display both, but do not silently
// write them as a single text-token rate or a fixed per-request image charge.
func parseOpenAIImages(raw []byte) ([]Price, error) {
	start := strings.Index(string(raw), "\nImage generation models\n")
	if start < 0 {
		return nil, errors.New("OpenAI 官方图片价格章节缺失")
	}
	rows, err := firstPriceTable(string(raw)[start:], []string{"Model", "Modality", "Input", "Cached input", "Output"})
	if err != nil {
		return nil, err
	}
	type imagePrice struct{ text, image *Rates }
	byModel := map[string]*imagePrice{}
	var names []string
	for _, row := range rows {
		if !publishedID.MatchString(row[0]) || ProviderID(row[0]) != "openai" {
			return nil, errors.New("OpenAI 官方图片模型名称无效")
		}
		entry := byModel[row[0]]
		if entry == nil {
			entry = &imagePrice{}
			byModel[row[0]] = entry
			names = append(names, row[0])
		}
		rates, err := openAIRates([]string{row[2], row[3], "-", row[4]})
		if err != nil {
			return nil, err
		}
		var target **Rates
		switch row[1] {
		case "Text":
			target = &entry.text
		case "Image":
			target = &entry.image
		default:
			return nil, errors.New("OpenAI 官方图片计费模态已变更")
		}
		if *target != nil {
			return nil, errors.New("OpenAI 官方图片计费模态重复")
		}
		*target = &rates
	}
	var prices []Price
	for _, name := range names {
		entry := byModel[name]
		if entry.text == nil || entry.image == nil || entry.text.InputPrice == "" || entry.image.InputPrice == "" || entry.image.OutputPrice == "" || entry.text.CacheReadPrice == "" || entry.image.CacheReadPrice == "" {
			return nil, errors.New("OpenAI 官方图片分模态单价不完整")
		}
		scope := "USD／百万 Token；Standard；文本输入 " + perMillion(entry.text.InputPrice) + "，文本缓存读取 " + perMillion(entry.text.CacheReadPrice) + "；图片输入 " + perMillion(entry.image.InputPrice) + "，图片缓存读取 " + perMillion(entry.image.CacheReadPrice) + "，图片输出 " + perMillion(entry.image.OutputPrice)
		prices = append(prices, Price{Model: name, Mode: "image_generation", Rates: *entry.text, ImageInputPrice: entry.image.InputPrice, ImageOutputPrice: entry.image.OutputPrice, ImageOutputUnit: "token", Scope: scope, SyncError: "该模型按文本和图片分别计价，暂不支持自动同步；请按官方模态配置计费"})
	}
	return prices, nil
}
