package officialpricing_test

import (
	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile("testdata/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func find(t *testing.T, ps []pricing.Price, name string) pricing.Price {
	t.Helper()
	for _, p := range ps {
		if strings.EqualFold(p.Model, name) {
			return p
		}
	}
	t.Fatalf("missing %s", name)
	return pricing.Price{}
}
func TestKimiPublishedNumbersPreserved(t *testing.T) {
	ps, e := pricing.ParseKimi(fixture(t, "kimi.md"))
	if e != nil {
		t.Fatal(e)
	}
	p := find(t, ps, "kimi-k3")
	if p.InputPrice != "0.00002" || p.OutputPrice != "0.0001" || p.CacheReadPrice != "0.000002" {
		t.Fatalf("wrong rates: %+v", p)
	}
}
func TestMiniMaxStandardDiscountAndContextTiers(t *testing.T) {
	ps, e := pricing.ParseMiniMax(fixture(t, "minimax.md"))
	if e != nil {
		t.Fatal(e)
	}
	p := find(t, ps, "MiniMax-M3")
	if p.InputPrice != "0.0000021" || len(p.Tiers) != 2 || p.Tiers[0].Condition != "len <= 512000" || p.Tiers[1].OutputPrice != "0.0000168" {
		t.Fatalf("wrong standard tiers: %+v", p)
	}
	p = find(t, ps, "MiniMax-M2.7")
	if p.CacheWritePrice != "0.000002625" {
		t.Fatalf("missing cache write: %+v", p)
	}
}
func TestGLMOutputLengthAndInclusiveContextBoundary(t *testing.T) {
	ps, e := pricing.ParseGLM(fixture(t, "glm.md"))
	if e != nil {
		t.Fatal(e)
	}
	p := find(t, ps, "glm-4.7")
	if len(p.Tiers) != 3 || !strings.Contains(p.BillingExpr, "c < 200") || !strings.Contains(p.BillingExpr, "len < 32000") {
		t.Fatalf("lost conditions: %+v", p)
	}
	p = find(t, ps, "glm-4.7-flash")
	if p.InputPrice != "0" || p.OutputPrice != "0" {
		t.Fatalf("free parsed incorrectly: %+v", p)
	}
}
func TestQwenBeijingDiscountModesAndNoAudioFlattening(t *testing.T) {
	ps, e := pricing.ParseQwen(fixture(t, "qwen.html"))
	if e != nil {
		t.Fatal(e)
	}
	p := find(t, ps, "qwen3.8-max")
	if p.InputPrice != "0.000012" {
		t.Fatalf("wrong region: %+v", p)
	}
	p = find(t, ps, "qwen3.7-plus")
	if p.InputPrice != "0.0000016" || p.OutputPrice != "0.0000064" {
		t.Fatalf("lost discount: %+v", p)
	}
	p = find(t, ps, "qwen-turbo")
	if !strings.Contains(p.BillingExpr, `param("enable_thinking") != true`) || len(p.Tiers) != 2 {
		t.Fatalf("lost mode pricing: %+v", p)
	}
	for _, p := range ps {
		if strings.Contains(p.Model, "omni") {
			t.Fatal("audio prices flattened")
		}
	}
}
func TestChangedOfficialFormatsReturnErrors(t *testing.T) {
	for name, parse := range map[string]func([]byte) ([]pricing.Price, error){"kimi": pricing.ParseKimi, "minimax": pricing.ParseMiniMax, "glm": pricing.ParseGLM, "qwen": pricing.ParseQwen} {
		t.Run(name, func(t *testing.T) {
			if _, e := parse([]byte("<html>maintenance</html>")); e == nil {
				t.Fatal("invalid page accepted")
			}
		})
	}
}

func TestKimiChangedColumnOrderIsRejected(t *testing.T) {
	raw := string(fixture(t, "kimi.md"))
	raw = strings.Replace(raw, `title: "输出价格"`, `title: "SWAP"`, 1)
	raw = strings.Replace(raw, `title: "输入价格（缓存命中）"`, `title: "输出价格"`, 1)
	raw = strings.Replace(raw, `title: "SWAP"`, `title: "输入价格（缓存命中）"`, 1)
	if _, err := pricing.ParseKimi([]byte(raw)); err == nil {
		t.Fatal("changed Kimi column order accepted")
	}
}
func TestMiniMaxMissingContextTierIsRejected(t *testing.T) {
	raw := string(fixture(t, "minimax.md"))
	lines := strings.Split(raw, "\n")
	kept := []string{}
	for _, line := range lines {
		if strings.Contains(line, "**MiniMax-M3**") && strings.Contains(line, "<br />>") {
			continue
		}
		kept = append(kept, line)
	}
	if _, err := pricing.ParseMiniMax([]byte(strings.Join(kept, "\n"))); err == nil {
		t.Fatal("incomplete context prices flattened to static price")
	}
}

func TestGLMMissingOutputTierIsRejected(t *testing.T) {
	raw := string(fixture(t, "glm.md"))
	lines := strings.Split(raw, "\n")
	kept := []string{}
	for _, line := range lines {
		if strings.Contains(line, "| GLM-4.7 ") && strings.Contains(line, "输出 ≥") {
			continue
		}
		kept = append(kept, line)
	}
	if _, err := pricing.ParseGLM([]byte(strings.Join(kept, "\n"))); err == nil {
		t.Fatal("missing output tier accepted")
	}
}
