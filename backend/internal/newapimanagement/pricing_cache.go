package newapimanagement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

type pricingCacheStore interface {
	ModelPricingCache(context.Context, string) (*configstore.ModelPricingCache, error)
	SaveModelPricingCache(context.Context, string, configstore.ModelPricingCache) error
}

type ModelPriceCatalog struct {
	Models        []Sub2APIModelPrice `json:"models"`
	MissingModels []string            `json:"missing_models"`
	FetchedAt     string              `json:"fetched_at"`
	ExpiresAt     string              `json:"expires_at"`
	Stale         bool                `json:"stale"`
	Warning       string              `json:"warning,omitempty"`
}

func pricingCacheFresh(at, now time.Time) bool {
	return !at.IsZero() && !at.After(now) && now.Sub(at) < 24*time.Hour
}

func catalogCacheKey(platform configstore.NewAPIPlatform, target configstore.TargetSettings) string {
	// A changed management target must never reuse another instance's prices.
	digest := sha256.Sum256([]byte(platform.ID + "\n" + strings.TrimRight(platform.BaseURL, "/") + "\n" + strings.TrimRight(target.BaseURL, "/")))
	return "catalog-v4:" + hex.EncodeToString(digest[:])
}

func (s *Service) ModelPriceCatalog(ctx context.Context, platformID string, force bool) (ModelPriceCatalog, error) {
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return ModelPriceCatalog{}, err
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return ModelPriceCatalog{}, err
	}
	store, ok := s.private.(pricingCacheStore)
	if !ok {
		return ModelPriceCatalog{}, errors.New("本地价格缓存存储尚未就绪")
	}
	s.pricingMu.Lock()
	defer s.pricingMu.Unlock()
	key := catalogCacheKey(*platform, target)
	cached, err := store.ModelPricingCache(ctx, key)
	if err != nil {
		return ModelPriceCatalog{}, err
	}
	previous := ModelPriceCatalog{}
	if cached != nil {
		if err := json.Unmarshal([]byte(cached.Content), &previous); err != nil {
			return ModelPriceCatalog{}, errors.New("本地价格缓存不可读")
		}
		age := time.Since(cached.FetchedAt)
		expires, _ := time.Parse(time.RFC3339Nano, previous.ExpiresAt)
		if !force && age >= 0 && ((previous.Stale && age < time.Minute) || (!previous.Stale && time.Now().Before(expires))) {
			return previous, nil
		}
	}
	// Bound aggregate fallback lookup time, including retries, to a normal read request.
	lookupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var prices []Sub2APIModelPrice
	var source RemotePricingSource
	var remoteErr, namesErr error
	var names []string
	var reads sync.WaitGroup
	reads.Go(func() { prices, source, remoteErr = s.loadRemotePricing(lookupCtx, force) })
	reads.Go(func() {
		namesCtx, stop := context.WithTimeout(lookupCtx, 8*time.Second)
		defer stop()
		names, namesErr = s.pricingModelNames(namesCtx, *platform)
	})
	reads.Wait()
	oldest := time.Now().UTC()
	if at, err := time.Parse(time.RFC3339Nano, source.FetchedAt); err == nil && at.Before(oldest) {
		oldest = at
	}
	warnings := []string{}
	if remoteErr != nil || source.Stale {
		warnings = append(warnings, "远程价卡刷新失败，已保留可用缓存；请稍后手动刷新")
	}
	byModel := map[string]Sub2APIModelPrice{}
	for _, price := range prices {
		price.Source = "remote"
		byModel[price.Model] = price
	}
	if remoteErr != nil {
		for _, price := range previous.Models {
			if price.Source == "remote" {
				byModel[price.Model] = price
			}
		}
	}
	if namesErr != nil {
		warnings = append(warnings, "平台模型目录读取失败，已保留可用缓存；请检查平台连接后刷新")
	}
	officialExpiry := s.mergeOfficialPricing(lookupCtx, store, byModel, names, previous, force, &warnings)
	missing := []string{}
	var fallbackErr error
	var client *adminclient.Client
	for _, name := range names {
		if _, found := byModel[name]; found {
			continue
		}
		if officialpricing.ProviderID(name) != "" {
			missing = append(missing, name)
			continue
		}
		if client == nil {
			client, fallbackErr = adminclient.New(adminclient.Config{BaseURL: target.BaseURL, AdminKey: target.AdminKey, Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 2}, s.client.Transport)
			if fallbackErr != nil {
				break
			}
		}
		var price adminclient.DefaultModelPricing
		var at time.Time
		price, at, fallbackErr = loadDefaultPricing(lookupCtx, store, client, key, name, force)
		if fallbackErr != nil {
			break
		}
		if at.Before(oldest) {
			oldest = at
		}
		if !price.Found {
			missing = append(missing, name)
			continue
		}
		byModel[name] = defaultPriceToModel(name, price)
	}
	if fallbackErr != nil {
		warnings = append(warnings, "Sub2API 默认价格读取失败，已保留可用缓存；请检查管理地址、密钥及接口版本后刷新")
	}
	// Failed lookups must not erase last-known prices or turn failures into 'not found'.
	if namesErr != nil || fallbackErr != nil {
		confirmedMissing := map[string]bool{}
		for _, name := range missing {
			confirmedMissing[name] = true
		}
		for _, price := range previous.Models {
			if _, found := byModel[price.Model]; !found && !confirmedMissing[price.Model] && (officialpricing.ProviderID(price.Model) == "" || price.Source == "official") {
				byModel[price.Model] = price
			}
		}
	}
	if len(byModel) == 0 && len(warnings) > 0 {
		return ModelPriceCatalog{}, errors.New(strings.Join(warnings, "；"))
	}
	now := time.Now().UTC()
	result := ModelPriceCatalog{Models: make([]Sub2APIModelPrice, 0, len(byModel)), MissingModels: missing, FetchedAt: oldest.Format(time.RFC3339Nano), ExpiresAt: oldest.Add(24 * time.Hour).Format(time.RFC3339Nano), Stale: len(warnings) > 0, Warning: strings.Join(warnings, "；")}
	if !officialExpiry.IsZero() && officialExpiry.Before(oldest.Add(24*time.Hour)) {
		result.ExpiresAt = officialExpiry.Format(time.RFC3339Nano)
	}
	if result.Stale && previous.FetchedAt != "" {
		result.FetchedAt, result.ExpiresAt = previous.FetchedAt, previous.ExpiresAt
	}
	for _, price := range byModel {
		result.Models = append(result.Models, price)
	}
	sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].Model < result.Models[j].Model })
	encoded, err := json.Marshal(result)
	if err != nil {
		return ModelPriceCatalog{}, err
	}
	if err := store.SaveModelPricingCache(ctx, key, configstore.ModelPricingCache{Content: string(encoded), FetchedAt: now}); err != nil {
		return ModelPriceCatalog{}, err
	}
	return result, nil
}

func (s *Service) pricingModelNames(ctx context.Context, platform configstore.NewAPIPlatform) ([]string, error) {
	options, err := s.readOptions(ctx, platform)
	if err != nil {
		return nil, err
	}
	configured, err := decodeConfiguredModels(options)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(configured))
	for _, price := range configured {
		names = append(names, price.Model)
	}
	payload, err := s.request(ctx, platform, http.MethodGet, "/api/channel/models_enabled", nil)
	if err == nil {
		names = append(names, decodeModelNames(payload)...)
	}
	return normalizeModels(names), err
}

func loadDefaultPricing(ctx context.Context, store pricingCacheStore, client *adminclient.Client, catalogKey, model string, force bool) (adminclient.DefaultModelPricing, time.Time, error) {
	digest := sha256.Sum256([]byte(model))
	key := catalogKey + ":model:" + hex.EncodeToString(digest[:])
	cached, err := store.ModelPricingCache(ctx, key)
	if err != nil {
		return adminclient.DefaultModelPricing{}, time.Time{}, err
	}
	if cached != nil && !force && pricingCacheFresh(cached.FetchedAt, time.Now()) {
		var price adminclient.DefaultModelPricing
		err := json.Unmarshal([]byte(cached.Content), &price)
		return price, cached.FetchedAt, err
	}
	price, err := client.ModelPricing(ctx, model)
	if err != nil {
		return price, time.Time{}, err
	}
	raw, err := json.Marshal(price)
	if err != nil {
		return price, time.Time{}, err
	}
	at := time.Now().UTC()
	err = store.SaveModelPricingCache(ctx, key, configstore.ModelPricingCache{Content: string(raw), FetchedAt: at})
	return price, at, err
}

func defaultPriceToModel(name string, price adminclient.DefaultModelPricing) Sub2APIModelPrice {
	item := Sub2APIModelPrice{Model: name, Source: "sub2api", InputPrice: price.InputPrice, OutputPrice: price.OutputPrice, CacheWritePrice: price.CacheWritePrice, CacheWrite1hPrice: price.CacheWrite1hPrice, CacheReadPrice: price.CacheReadPrice, ImageInputPrice: price.ImageInputPrice, ImageOutputPrice: price.ImageOutputPrice}
	item.ModelRatio, item.CompletionRatio, _ = sub2APIRatios(item.InputPrice, item.OutputPrice)
	item.CacheRatio = priceRatio(item.InputPrice, item.CacheReadPrice)
	item.CreateCacheRatio = priceRatio(item.InputPrice, item.CacheWritePrice)
	item.CreateCache1hRatio = priceRatio(item.InputPrice, item.CacheWrite1hPrice)
	item.ImageRatio = priceRatio(item.InputPrice, item.ImageInputPrice)
	return item
}

func (s *Service) loadRemotePricing(ctx context.Context, force bool) ([]Sub2APIModelPrice, RemotePricingSource, error) {
	store, ok := s.private.(pricingCacheStore)
	if !ok {
		return nil, RemotePricingSource{}, errors.New("本地价格缓存存储尚未就绪")
	}
	key := "remote-v1:" + defaultSub2APIPricingURL
	cached, err := store.ModelPricingCache(ctx, key)
	if err != nil {
		return nil, RemotePricingSource{}, err
	}
	if cached != nil && !force && pricingCacheFresh(cached.FetchedAt, time.Now()) {
		prices, err := decodeSub2APIPricingJSON([]byte(cached.Content))
		return prices, buildRemotePricingSource([]byte(cached.Content), cached.FetchedAt), err
	}
	remoteCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	prices, raw, err := s.fetchRemotePricingCatalog(remoteCtx)
	if err == nil && len(prices) == 0 {
		err = errors.New("远程价卡未返回有效价格")
	}
	if err != nil {
		if cached == nil {
			return nil, RemotePricingSource{}, err
		}
		prices, decodeErr := decodeSub2APIPricingJSON([]byte(cached.Content))
		source := buildRemotePricingSource([]byte(cached.Content), cached.FetchedAt)
		source.Stale, source.Warning = true, "远程价卡刷新失败，当前显示上次成功缓存；请稍后刷新"
		return prices, source, decodeErr
	}
	now := time.Now().UTC()
	if err := store.SaveModelPricingCache(ctx, key, configstore.ModelPricingCache{Content: string(raw), FetchedAt: now}); err != nil {
		return nil, RemotePricingSource{}, err
	}
	return prices, buildRemotePricingSource(raw, now), nil
}
