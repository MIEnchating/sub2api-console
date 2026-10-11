package modelcheck_test

import (
	"context"
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestAnimationMarkdownExtractsDocumentAndKeepsOriginalResponse(t *testing.T) {
	pasted, err := os.ReadFile("testdata/pelican-markdown.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, source, marker string
		svg                  bool
	}{
		{"pasted pelican with prose and JavaScript", string(pasted), "requestAnimationFrame(animate)", false},
		{"HTML fence with surrounding prose", "说明\n```html\n" + htmlAnimation + "\n```\n效果说明", "@keyframes", false},
		{"SVG fence with surrounding prose", "说明\n```svg\n" + fixtureSVG + "\n```\n效果说明", "animateTransform", true},
		{"unlabelled fence", "说明\n```\n" + htmlAnimation + "\n```\n效果说明", "@keyframes", false},
		{"tilde fence and CRLF", "说明\r\n~~~HTML\r\n" + htmlAnimation + "\r\n~~~\r\n效果说明", "@keyframes", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"output_text": tc.source})
			})
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			if row.Status != "succeeded" {
				t.Fatalf("generation rejected: %s", row.Error)
			}
			if row.Source != tc.source {
				t.Fatal("original response changed")
			}
			rendered := row.HTML
			if tc.svg {
				rendered = row.SVG
			} else if !strings.Contains(rendered, "Content-Security-Policy") {
				t.Fatal("missing preview CSP")
			}
			if !strings.Contains(rendered, tc.marker) || strings.Contains(rendered, "效果说明") {
				t.Fatal("document lost content or included surrounding prose")
			}
		})
	}
}

func TestAnimationMarkdownRejectsAmbiguousIncompleteAndUnsafeDocuments(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"two documents", "```html\n" + htmlAnimation + "\n```\n```html\n" + htmlAnimation + "\n```"},
		{"unclosed fence", "说明\n```html\n" + htmlAnimation},
		{"incomplete document", "说明\n```html\n<html><body><svg></svg>\n```\n说明"},
		{"external script", "说明\n```html\n<html><head></head><body><script src=\"https://example.invalid/a.js\"></script></body></html>\n```\n说明"},
		{"external image", "说明\n```html\n<html><body><img src=\"https://example.invalid/a.png\"></body></html>\n```\n说明"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"output_text": tc.source})
			})
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			if row.Status != "failed" || row.HTML != "" || row.SVG != "" {
				t.Fatal("invalid content became renderable")
			}
			if row.Source != tc.source {
				t.Fatal("failed response source was not retained")
			}
		})
	}
}
