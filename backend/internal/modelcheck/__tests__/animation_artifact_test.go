package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

const htmlAnimation = `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><style>body{margin:0;background:#eef}svg{width:100%;height:auto}.wheel{animation:spin 2s linear infinite;transform-origin:center;transform-box:fill-box}@keyframes spin{to{transform:rotate(360deg)}}@media(max-width:600px){svg{max-height:80vh}}</style></head><body><svg viewBox="0 0 640 400"><circle class="wheel" r="20" cx="100" cy="100"/></svg></body></html>`
const htmlAnimationWithFilter = `<!DOCTYPE html><html><head><style>svg{width:100%}</style></head><body><svg viewBox="0 0 100 100"><defs><filter id="shadow"><feGaussianBlur stdDeviation="2"/><feOffset dx="1" dy="1"/></filter><marker id="marker" markerWidth="5" markerHeight="5" refX="1" refY="1" orient="auto"><path d="M0 0L2 1L0 2"/></marker><symbol id="wheel" viewBox="0 0 20 20"><circle r="9" cx="10" cy="10"/></symbol><linearGradient id="paint"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient></defs><use href="#wheel" width="20" height="20"/><g filter="url(#shadow)" marker-end="url(#marker)"><circle cx="50" cy="50" r="20" fill="url(#paint)"/></g></svg></body></html>`
const requestedAnimationPrompt = `请生成可直接运行的单文件HTML，使用内联SVG绘制鹈鹕骑自行车的二维循环动画。画面以鹈鹕和自行车为主体，展示清晰的身体结构、踩踏动作和车轮转动，配合协调的背景、配色与层次。动画应流畅自然、衔接连续，并适配不同屏幕尺寸。禁止依赖外部资源，只输出完整HTML，不要代码围栏或解释文字。`

func TestAnimationUsesSelectedModelLowEffortAndHTMLPrompt(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model     string
			Input     string
			Reasoning struct{ Effort string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "test-model" || body.Reasoning.Effort != "low" || body.Input != requestedAnimationPrompt {
			t.Errorf("unexpected model, effort or prompt: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "gpt-6-astra", "output_text": htmlAnimation, "usage": map[string]int{"input_tokens": 100, "output_tokens": 200, "total_tokens": 300}})
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
	raw, _ := json.Marshal(row)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	if task.Status != "succeeded" || result["html"] == nil || result["source"] != htmlAnimation || result["prompt"] != requestedAnimationPrompt || result["reasoning_effort"] != "low" || result["model"] != "test-model" {
		t.Fatalf("missing HTML metadata: %s", raw)
	}
	usage, _ := result["usage"].(map[string]any)
	if usage["input_tokens"] != float64(100) || usage["output_tokens"] != float64(200) || usage["total_tokens"] != float64(300) {
		t.Fatalf("missing usage: %s", raw)
	}
}

func TestAnimationReadsChatUsageAfterFinishReason(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/responses" {
			w.WriteHeader(404)
			return
		}
		var body struct {
			Model           string
			ReasoningEffort string `json:"reasoning_effort"`
			StreamOptions   struct {
				IncludeUsage bool `json:"include_usage"`
			} `json:"stream_options"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "test-model" || body.ReasoningEffort != "low" || !body.StreamOptions.IncludeUsage {
			t.Errorf("invalid chat parameters: %+v", body)
		}
		_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":34,\"total_tokens\":46}}\n\ndata: [DONE]\n\n", fixtureSVG)
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(finished(t, f).Result["animations"])
	if !strings.Contains(string(raw), `"output_tokens":34`) {
		t.Fatalf("lost trailing usage: %s", raw)
	}
}

func TestHTMLAnimationAllowsCommonInlineSVGEffects(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": htmlAnimationWithFilter})
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
	if task.Status != "succeeded" || row.HTML == "" || row.SVG != "" || row.Source != htmlAnimationWithFilter {
		t.Fatalf("HTML scene was rejected or converted to SVG: %#v", row)
	}
	for _, content := range []string{"feGaussianBlur", "feOffset", "markerWidth", "symbol", `href="#wheel"`, "Content-Security-Policy", "script-src"} {
		if !strings.Contains(row.HTML, content) {
			t.Errorf("HTML lost %q", content)
		}
	}
}

func TestHTMLAnimationRemovesScriptsAndEventHandlersFromPreview(t *testing.T) {
	source := strings.Replace(htmlAnimation, "</body>", `<script>window.alert("never run")</script><div onclick="window.alert(1)">安全内容</div></body>`, 1)
	f := setup(t, 1, "openai", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": source})
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Status != "succeeded" || row.HTML == "" || row.Source != source {
		t.Fatalf("script-bearing scene was not rendered safely: %#v", row)
	}
	if strings.Contains(strings.ToLower(row.HTML), "<script") || strings.Contains(row.HTML, "onclick") || !strings.Contains(row.HTML, "安全内容") || !strings.Contains(row.HTML, "script-src &#39;none&#39;") {
		t.Fatalf("unsafe content remained in renderable HTML: %s", row.HTML)
	}
}

func TestHTMLAnimationRejectsActiveContentAndExternalResources(t *testing.T) {
	for _, content := range []string{`<img src="https://example.invalid/a">`, `<meta http-equiv="refresh" content="0;url=https://example.invalid">`, `<style>@import 'https://example.invalid/x';</style>`, `<style>body{background:url(https://example.invalid/x)}</style>`, `<style>body{background:u\72l(https://example.invalid/x)}</style>`, `<svg><animate attributeName="href" to="https://example.invalid"/></svg>`} {
		t.Run(content, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"output_text": strings.Replace(htmlAnimation, "</body>", content+"</body>", 1)})
			})
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			if task.Status != "failed" {
				t.Fatal("unsafe HTML accepted")
			}
		})
	}
}

func TestHTMLAnimationEffectsStillRejectExternalResourcesAndScripts(t *testing.T) {
	for _, content := range []string{
		`<filter id="shadow"><feImage href="https://example.invalid/image"/></filter>`,
		`<g filter="url(https://example.invalid/filter)"/>`,
		`<path marker-end="url(https://example.invalid/marker)"/>`,
		`<use href="https://example.invalid/symbol"/>`,
		`<filter><feGaussianBlur><animate attributeName="href" to="https://example.invalid"/></feGaussianBlur></filter>`,
	} {
		t.Run(content, func(t *testing.T) {
			source := strings.Replace(htmlAnimationWithFilter, "</svg>", content+"</svg>", 1)
			f := setup(t, 1, "openai", func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"output_text": source})
			})
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
			if task.Status != "failed" || row.HTML != "" || row.SVG != "" {
				t.Fatalf("unsafe scene accepted: %#v", row)
			}
		})
	}
}

func TestHTMLAnimationReportsRejectedElementWithoutItsContent(t *testing.T) {
	source := strings.Replace(htmlAnimationWithFilter, "</svg>", `<feImage href="https://example.invalid/private-path"/></svg>`, 1)
	f := setup(t, 1, "openai", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": source})
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if !strings.Contains(row.Error, "外部资源") || strings.Contains(row.Error, "private-path") {
		t.Fatalf("missing bounded element diagnostic: %q", row.Error)
	}
	if row.Source != source {
		t.Fatalf("rejected source was not retained: length=%d", len(row.Source))
	}
}

func TestHTMLAnimationBoundsRejectedSource(t *testing.T) {
	source := strings.Replace(htmlAnimation, "</body>", `<script>`+strings.Repeat("中", 50000)+`</script></body>`, 1)
	f := setup(t, 1, "openai", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": source})
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Status != "failed" || !row.SourceTruncated || len(row.Source) > 128<<10 || !strings.HasPrefix(source, row.Source) {
		t.Fatalf("rejected source was not bounded: status=%s truncated=%v length=%d", row.Status, row.SourceTruncated, len(row.Source))
	}
	if row.HTML != "" || row.SVG != "" {
		t.Fatal("rejected source became renderable")
	}
}
