package newapimanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
	"strings"
	"sync"
	"time"
)

type officialResult struct {
	provider officialpricing.Provider
	prices   []officialpricing.Price
	at       time.Time
	err      error
}

func (s *Service) mergeOfficialPricing(ctx context.Context, store pricingCacheStore, byModel map[string]Sub2APIModelPrice, names []string, previous ModelPriceCatalog, force bool, warnings *[]string) time.Time {
	needed := map[string]bool{}
	requested := map[string][]string{}
	record := func(name string) {
		id := officialpricing.ProviderID(name)
		if id != "" {
			needed[id] = true
			key := strings.ToLower(name)
			requested[key] = append(requested[key], name)
		}
	}
	for name := range byModel {
		record(name)
		if officialpricing.ProviderID(name) != "" {
			delete(byModel, name)
		}
	}
	for _, name := range names {
		record(name)
	}
	for _, p := range previous.Models {
		record(p.Model)
	}
	results := make([]officialResult, len(officialpricing.Providers))
	var wg sync.WaitGroup
	for i, p := range officialpricing.Providers {
		if needed[p.ID] {
			wg.Go(func() { results[i] = s.loadOfficialPricing(ctx, store, p, force) })
		}
	}
	wg.Wait()
	var expiry time.Time
	for _, r := range results {
		if r.provider.ID == "" {
			continue
		}
		if r.err != nil {
			*warnings = append(*warnings, fmt.Sprintf("%s 官方价格刷新失败（%v），已保留上次成功的官方价格；请检查网络或官网后重试", r.provider.Name, r.err))
			if len(r.prices) == 0 {
				for _, p := range previous.Models {
					if p.Source == "official" && officialpricing.ProviderID(p.Model) == r.provider.ID {
						byModel[p.Model] = p
					}
				}
			}
		}
		for _, p := range r.prices {
			item := Sub2APIModelPrice{Source: "official", SourceURL: p.SourceURL, SourceScope: p.Scope, Provider: r.provider.ID, Mode: "chat", Model: p.Model, InputPrice: p.InputPrice, OutputPrice: p.OutputPrice, CacheReadPrice: p.CacheReadPrice, CacheWritePrice: p.CacheWritePrice, PriceTiers: p.Tiers, BillingExpr: p.BillingExpr}
			if p.TimePricing.Timezone != "" {
				schedule := p.TimePricing
				item.TimePricing = &schedule
			}
			item.ModelRatio, item.CompletionRatio, _ = sub2APIRatios(item.InputPrice, item.OutputPrice)
			item.CacheRatio = priceRatio(item.InputPrice, item.CacheReadPrice)
			item.CreateCacheRatio = priceRatio(item.InputPrice, item.CacheWritePrice)
			byModel[item.Model] = item
			// Case variants of the exact published ID are allowed; no dated aliases or
			// reseller prefixes are inferred from family names.
			for _, name := range requested[strings.ToLower(item.Model)] {
				alias := item
				alias.Model = name
				byModel[name] = alias
			}
		}
		if !r.at.IsZero() {
			at := r.at.Add(time.Hour)
			if expiry.IsZero() || at.Before(expiry) {
				expiry = at
			}
		}
	}
	return expiry
}
func (s *Service) loadOfficialPricing(ctx context.Context, store pricingCacheStore, p officialpricing.Provider, force bool) officialResult {
	r := officialResult{provider: p}
	key := "official-v2:" + p.URL
	cached, err := store.ModelPricingCache(ctx, key)
	if err != nil {
		r.err = err
		return r
	}
	if cached != nil && json.Unmarshal([]byte(cached.Content), &r.prices) == nil && len(r.prices) > 0 {
		r.at = cached.FetchedAt
	}
	fresh := !r.at.IsZero() && !r.at.After(time.Now()) && time.Since(r.at) < time.Hour
	if !force && fresh {
		return r
	}
	ps, err := officialpricing.Fetch(ctx, s.client, p)
	if err == nil {
		raw, e := json.Marshal(ps)
		err = e
		if err == nil {
			at := time.Now().UTC()
			err = store.SaveModelPricingCache(ctx, key, configstore.ModelPricingCache{Content: string(raw), FetchedAt: at})
			if err == nil {
				r.prices = ps
				r.at = at
			}
		}
	}
	r.err = err
	return r
}
