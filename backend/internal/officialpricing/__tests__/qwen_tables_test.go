package officialpricing_test

import (
	"strings"
	"testing"

	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

const qwenTextTable = `<table><tr><th>模型 ID（Model ID）</th><th>输入单价（每百万Token）</th><th>输出单价（每百万Token）</th></tr><tr><td>qwen-turbo</td><td>0.3元</td><td>0.6元</td></tr></table>`

func TestQwenUnrelatedTableWithOverflowingRowspanDoesNotBlockTextPrices(t *testing.T) {
	unrelated := string(fixture(t, "qwen-unrelated-table.html"))
	for _, position := range []string{"before", "after"} {
		t.Run(position, func(t *testing.T) {
			raw := unrelated + qwenTextTable
			if position == "after" {
				raw = qwenTextTable + unrelated
			}
			prices, err := pricing.ParseQwen([]byte(`<h4>华北2（北京）</h4>` + raw))
			if err != nil {
				t.Fatal(err)
			}
			p := find(t, prices, "qwen-turbo")
			if len(prices) != 1 || p.InputPrice != "0.0000003" || p.OutputPrice != "0.0000006" {
				t.Fatalf("unrelated table changed Qwen prices: %+v", prices)
			}
		})
	}
}

func TestQwenUnsupportedModalityWithOverflowingRowspanDoesNotBlockTextPrices(t *testing.T) {
	raw := strings.Replace(qwenTextTable, "qwen-turbo", "qwen-audio-turbo", 1)
	raw = strings.Replace(raw, "<td>0.3元</td>", `<td rowspan="3">0.3元</td>`, 1)
	prices, err := pricing.ParseQwen([]byte(`<h4>华北2（北京）</h4>` + raw + qwenTextTable))
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 1 || prices[0].Model != "qwen-turbo" {
		t.Fatalf("unsupported modality was included: %+v", prices)
	}
}

func TestQwenSupportedTextTableWithOverflowingRowspanRejectsPartialPrices(t *testing.T) {
	invalid := strings.Replace(qwenTextTable, "qwen-turbo", "qwen-plus", 1)
	invalid = strings.Replace(invalid, "<td>0.3元</td>", `<td rowspan="3">0.3元</td>`, 1)
	prices, err := pricing.ParseQwen([]byte(`<h4>华北2（北京）</h4>` + qwenTextTable + invalid))
	if err == nil || !strings.Contains(err.Error(), "官方价格表超出边界") || len(prices) != 0 {
		t.Fatalf("incomplete text table returned partial prices: prices=%+v err=%v", prices, err)
	}
}
