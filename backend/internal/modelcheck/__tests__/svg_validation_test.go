package modelcheck_test

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func generateSVGFixture(t *testing.T, svg string) modelcheck.AnimationResult {
	t.Helper()
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"output_text": svg})
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	return task.Result["animations"].([]modelcheck.AnimationResult)[0]
}

func TestSVGAcceptsCommonPresentationAndMetadata(t *testing.T) {
	cases := []struct{ name, svg, preserved string }{
		{"version and accessible metadata", `<svg xmlns="http://www.w3.org/2000/svg" version="1.1" role="img" aria-labelledby="title" class="scene"><title id="title">骑车动画</title><path d="M0 0L20 20"/></svg>`, `version="1.1"`},
		{"stroke and geometry presentation", `<svg><path d="M0 0L20 20" stroke-miterlimit="4" vector-effect="non-scaling-stroke" paint-order="stroke fill" shape-rendering="geometricPrecision"/></svg>`, `stroke-miterlimit="4"`},
		{"xml whitespace", `<svg xml:space="preserve"><text x="10" y="20">a b</text></svg>`, `xml:space="preserve"`},
		{"static inline style", `<svg><g style="fill: #cceeff; stroke: #222; stroke-width:2; transform-origin:50% 50%; transform-box:fill-box"><circle r="10"><animateTransform attributeName="transform" type="rotate" from="0" to="360" dur="2s" repeatCount="indefinite"/></circle></g></svg>`, `transform-box:fill-box`},
		{"colors and transforms in inline style", `<svg><circle r="10" style="fill:rgb(20, 50, 80); opacity:0.8; transform:translate(2px, 3px) rotate(10deg)"/></svg>`, `rgb(20, 50, 80)`},
		{"local CSS references", `<svg><defs><linearGradient id="paint"><stop stop-color="red"/></linearGradient></defs><circle r="10" style="fill:url('#paint')"/></svg>`, `url(`},
		{"local animated paints", `<svg><defs><linearGradient id="paint"><stop stop-color="red"/></linearGradient></defs><circle r="10"><animate attributeName="fill" values="url(#paint);blue" dur="2s"/></circle></svg>`, `values="url(#paint);blue"`},
		{"matching legacy href", `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><defs><circle id="wheel" r="10"/></defs><use href="#wheel" xlink:href="#wheel"/></svg>`, `href="#wheel"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateSVGFixture(t, tc.svg)
			if result.Status != "succeeded" || !strings.Contains(result.SVG, tc.preserved) {
				t.Fatalf("safe SVG rejected or altered: status=%s error=%s svg=%s", result.Status, result.Error, result.SVG)
			}
			decoder := xml.NewDecoder(strings.NewReader(result.SVG))
			for {
				_, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("invalid sanitized XML: %v", err)
				}
			}
		})
	}
}

func TestSVGStyleCompatibilityStillRejectsExecutableAndExternalContent(t *testing.T) {
	cases := []struct{ name, svg string }{
		{"external inline fill", `<svg><circle style="fill:url(https://example.invalid/paint)"/></svg>`},
		{"external escaped URL", `<svg><circle style="fill:u\72l(https://example.invalid/paint)"/></svg>`},
		{"CSS comments", `<svg><circle style="fill:u/**/rl(https://example.invalid/paint)"/></svg>`},
		{"unsafe CSS property", `<svg><circle style="behavior:url(#code)"/></svg>`},
		{"CSS expression", `<svg><circle style="fill:expression(alert(1))"/></svg>`},
		{"variable with remote fallback", `<svg><circle style="fill:var(--paint,url(https://example.invalid/paint))"/></svg>`},
		{"style block", `<svg><style>@import 'https://example.invalid/style';</style><circle/></svg>`},
		{"event with safe style", `<svg><circle style="fill:red" onload="alert(1)"/></svg>`},
		{"animated style", `<svg><circle><set attributeName="style" to="fill:red"/></circle></svg>`},
		{"external animated paint", `<svg><circle><animate attributeName="fill" values="red;url(https://example.invalid/paint)"/></circle></svg>`},
		{"duplicate attributes", `<svg><circle fill="red" fill="blue"/></svg>`},
		{"conflicting href", `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><use href="#one" xlink:href="#two"/></svg>`},
		{"foreign namespace", `<svg xmlns:evil="https://example.invalid"><circle evil:fill="red"/></svg>`},
		{"xml base", `<svg xml:base="https://example.invalid"><use href="#wheel"/></svg>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateSVGFixture(t, tc.svg)
			if result.Status != "failed" || result.SVG != "" || result.Error == "" {
				t.Fatalf("unsafe SVG accepted: %#v", result)
			}
		})
	}
}

func TestSVGRejectedAttributeNamesAreReportedWithoutValues(t *testing.T) {
	result := generateSVGFixture(t, `<svg><circle unsupportedAttribute="private-value"/></svg>`)
	if !strings.Contains(result.Error, "unsupportedAttribute") || strings.Contains(result.Error, "private-value") {
		t.Fatalf("missing safe attribute diagnostic: %q", result.Error)
	}
}
