package officialpricing_test

import (
	"encoding/json"
	"fmt"
	"testing"

	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

func kimiCachePage(write5, write1h string) []byte {
	row, _ := json.Marshal([]string{"kimi-k3", "1M tokens", write5, write1h, "¥2", "¥20", "¥100", "1M tokens"})
	return []byte(fmt.Sprintf(`<DocTable columns={[
{title: "模型"}, {title: "计费单位"}, {title: "缓存写入（TTL 5min）"},
{title: "缓存写入（TTL 1h）"}, {title: "输入价格（缓存命中）"},
{title: "输入价格（缓存未命中）"}, {title: "输出价格"}, {title: "上下文窗口"}
]} rows={[%s]} />`, row))
}

func TestKimiInvalidCacheWritePriceRejectsIncompleteCatalog(t *testing.T) {
	for _, tc := range []struct{ name, short, long string }{
		{"invalid short amount", "¥invalid", "¥40"},
		{"invalid long amount", "¥20", "¥invalid"},
		{"wrong currency", "¥20", "$40"},
		{"empty value", "", "¥40"},
		{"missing long tier", "¥20", "-"},
		{"missing short tier", "—", "¥40"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if prices, err := pricing.ParseKimi(kimiCachePage(tc.short, tc.long)); err == nil || len(prices) != 0 {
				t.Fatalf("incomplete cache prices accepted: prices=%+v err=%v", prices, err)
			}
		})
	}
}

func TestKimiZeroCacheWritePricesRemainExplicitTiers(t *testing.T) {
	prices, err := pricing.ParseKimi(kimiCachePage("¥0", "¥0"))
	if err != nil {
		t.Fatal(err)
	}
	p := find(t, prices, "kimi-k3")
	if len(p.Tiers) != 2 || p.Tiers[0].CacheWritePrice != "0" || p.Tiers[1].CacheWritePrice != "0" {
		t.Fatalf("zero cache prices lost: %+v", p)
	}
}

func TestKimiExplicitlyUnsupportedCacheWritesKeepStandardPrices(t *testing.T) {
	prices, err := pricing.ParseKimi(kimiCachePage("-", "—"))
	if err != nil {
		t.Fatal(err)
	}
	p := find(t, prices, "kimi-k3")
	if p.CacheWritePrice != "" || len(p.Tiers) != 0 || p.InputPrice != "0.00002" {
		t.Fatalf("unsupported cache writes changed standard prices: %+v", p)
	}
}
