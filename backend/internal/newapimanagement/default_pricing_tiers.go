package newapimanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

type defaultPricingRates struct {
	Input        json.Number `json:"input_price"`
	Output       json.Number `json:"output_price"`
	CacheRead    json.Number `json:"cache_read_price"`
	CacheWrite   json.Number `json:"cache_write_price"`
	CacheWrite1h json.Number `json:"cache_write_1h_price"`
}

type defaultPricingInterval struct {
	Min   *int64 `json:"min_tokens"`
	Max   *int64 `json:"max_tokens"`
	Label string `json:"tier_label"`
	defaultPricingRates
}

type defaultPricingReference struct {
	defaultPricingRates
	Intervals []defaultPricingInterval `json:"intervals"`
}

type defaultPricingPlaza struct {
	Data struct {
		Groups []struct {
			Models []struct {
				Name     string                   `json:"name"`
				Official *defaultPricingReference `json:"official_pricing"`
			} `json:"models"`
		} `json:"groups"`
	} `json:"data"`
}

// The autofill endpoint omits billing tiers. Only the plaza's official reference
// is suitable here: its paid-price field includes channel/group modifiers.
func (s *Service) defaultPricingReferences(ctx context.Context, target configstore.TargetSettings) (map[string]defaultPricingReference, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	payload, err := s.requestUpstream(ctx, configstore.AuthRecord{BaseURL: target.BaseURL}, "/api/v1/model-plaza")
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var plaza defaultPricingPlaza
	if err := json.Unmarshal(raw, &plaza); err != nil || plaza.Data.Groups == nil {
		return nil, errors.New("Sub2API 模型广场参考价结构无效")
	}
	result := map[string]defaultPricingReference{}
	for _, group := range plaza.Data.Groups {
		for _, model := range group.Models {
			if model.Name == "" || model.Official == nil {
				continue
			}
			if previous, found := result[model.Name]; found && !reflect.DeepEqual(previous, *model.Official) {
				return nil, errors.New("Sub2API 模型广场同名模型的参考价不一致")
			}
			result[model.Name] = *model.Official
		}
	}
	return result, nil
}

func applyDefaultPricingReference(item Sub2APIModelPrice, reference defaultPricingReference) (Sub2APIModelPrice, error) {
	if err := validateDefaultRates(reference.defaultPricingRates); err != nil {
		return item, err
	}
	item.InputPrice, item.OutputPrice = reference.Input.String(), reference.Output.String()
	item.CacheReadPrice, item.CacheWritePrice, item.CacheWrite1hPrice = reference.CacheRead.String(), reference.CacheWrite.String(), reference.CacheWrite1h.String()
	item.ModelRatio, item.CompletionRatio, _ = sub2APIRatios(item.InputPrice, item.OutputPrice)
	item.CacheRatio = priceRatio(item.InputPrice, item.CacheReadPrice)
	item.CreateCacheRatio = priceRatio(item.InputPrice, item.CacheWritePrice)
	item.CreateCache1hRatio = priceRatio(item.InputPrice, item.CacheWrite1hPrice)
	item.ImageRatio = priceRatio(item.InputPrice, item.ImageInputPrice)
	if len(reference.Intervals) == 0 {
		return item, nil
	}
	intervals := append([]defaultPricingInterval(nil), reference.Intervals...)
	sort.SliceStable(intervals, func(i, j int) bool {
		if intervals[i].Min == nil || intervals[j].Min == nil {
			return false
		}
		return *intervals[i].Min < *intervals[j].Min
	})
	expressions := []string{}
	for index, interval := range intervals {
		if err := validateDefaultRates(interval.defaultPricingRates); err != nil {
			return item, err
		}
		if interval.Min == nil || *interval.Min < 0 || (index == 0 && *interval.Min != 0) ||
			(interval.Max != nil && *interval.Max <= *interval.Min) ||
			(index == len(intervals)-1 && interval.Max != nil) ||
			(index > 0 && (intervals[index-1].Max == nil || *intervals[index-1].Max != *interval.Min)) {
			return item, errors.New("Sub2API 默认价格阶梯范围不连续或无效")
		}
		condition := ""
		if interval.Max != nil {
			condition = "len <= " + strconv.FormatInt(*interval.Max, 10)
		}
		label := strings.TrimSpace(interval.Label)
		if label == "" {
			if interval.Max == nil {
				label = fmt.Sprintf("> %d Token", *interval.Min)
			} else {
				label = fmt.Sprintf("%d–%d Token", *interval.Min, *interval.Max)
			}
		}
		item.PriceTiers = append(item.PriceTiers, officialpricing.Tier{
			Label: label, Condition: condition, CacheWrite1hPrice: interval.CacheWrite1h.String(),
			Rates: officialpricing.Rates{InputPrice: interval.Input.String(), OutputPrice: interval.Output.String(), CacheReadPrice: interval.CacheRead.String(), CacheWritePrice: interval.CacheWrite.String()},
		})
		terms := []string{}
		for _, field := range [][2]string{{"p", interval.Input.String()}, {"c", interval.Output.String()}, {"cr", interval.CacheRead.String()}, {"cc", interval.CacheWrite.String()}, {"cc1h", interval.CacheWrite1h.String()}, {"img", item.ImageInputPrice}, {"img_o", item.ImageOutputPrice}} {
			if field[1] == "" {
				continue
			}
			value, ok := new(big.Rat).SetString(field[1])
			if !ok {
				return item, errors.New("Sub2API 默认价格数值无效")
			}
			value.Mul(value, big.NewRat(1000000, 1))
			decimal := strings.TrimRight(strings.TrimRight(value.FloatString(38), "0"), ".")
			if decimal == "" {
				decimal = "0"
			}
			terms = append(terms, field[0]+" * "+decimal)
		}
		expression := "tier(" + strconv.Quote(label) + ", " + strings.Join(terms, " + ") + ")"
		if condition != "" {
			expression = condition + " ? " + expression
		}
		expressions = append(expressions, expression)
	}
	item.BillingExpr = strings.Join(expressions, " : ")
	return item, nil
}

func validateDefaultRates(rates defaultPricingRates) error {
	if rates.Input == "" || rates.Output == "" {
		return errors.New("Sub2API 默认价格阶梯缺少输入或输出单价")
	}
	for _, value := range []json.Number{rates.Input, rates.Output, rates.CacheRead, rates.CacheWrite, rates.CacheWrite1h} {
		if value != "" && !validDecimal(value.String()) {
			return errors.New("Sub2API 默认价格阶梯包含无效单价")
		}
	}
	return nil
}
