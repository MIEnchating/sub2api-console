package animationquality_test

import (
	"encoding/json"
	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"testing"
)

func TestReviewRequiresMajorCoreEvidenceForSuspiciousLabel(t *testing.T) {
	const source = `<svg><g id="legs"/><g id="clouds"/></svg>`
	for _, tc := range []struct{ name, label, severity, scope, evidence, want string }{
		{"minor leg stretch stays normal", "normal", "minor", "motion", `<g id="legs"/>`, "normal"},
		{"background seam stays normal", "normal", "minor", "secondary", `<g id="clouds"/>`, "normal"},
		{"minor issue cannot justify suspicious", "suspicious", "minor", "motion", `<g id="legs"/>`, "unknown"},
		{"background alone cannot justify suspicious", "suspicious", "major", "secondary", `<g id="clouds"/>`, "unknown"},
		{"missing pedal linkage has core evidence", "suspicious", "major", "motion", `<g id="legs"/>`, "suspicious"},
		{"fabricated code is rejected", "suspicious", "major", "motion", `pedals.static = true`, "unknown"},
		{"major defect contradicts normal", "normal", "major", "motion", `<g id="legs"/>`, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"label": tc.label, "reason": "依据主要运动和实际影响判断", "basis": []q.ReviewBasis{{Requirement: "踩踏动作", Evidence: `<g id="legs"/>`, Assessment: "根据腿部实现判断踩踏是否成立"}}, "issues": []q.ReviewIssue{{Severity: tc.severity, Scope: tc.scope, Evidence: tc.evidence, Impact: "说明此片段对主体效果的影响"}}})
			got := q.ParseReview(string(raw), "展示踩踏动作", source)
			if got.Status != tc.want || got.RubricVersion != q.ReviewRubricVersion {
				t.Fatalf("%+v", got)
			}
			if tc.want == "unknown" && got.Reason != "review_insufficient_evidence" {
				t.Fatalf("reason: %+v", got)
			}
		})
	}
}

func TestReviewAcceptsCompleteCoreWithoutDefects(t *testing.T) {
	got := q.ParseReview(`{"label":"normal","reason":"主要结构有源码支持，细节待视觉复核","basis":[{"requirement":"内联SVG","evidence":"<svg/>","assessment":"使用内联SVG容器"}],"issues":[]}`, "使用内联SVG", `<svg/>`)
	if got.Status != "normal" || got.Reason != "" {
		t.Fatalf("%+v", got)
	}
}

func TestReviewRejectsIncompleteOrMalformedEvidence(t *testing.T) {
	for _, output := range []string{
		`{"label":"suspicious","reason":"很差"}`,
		`{"label":"suspicious","reason":"很差","basis":[{"requirement":"内联SVG","evidence":"<svg/>","assessment":"只有空容器"}],"issues":[]}`,
		`{"label":"normal","reason":"完整","basis":[{"requirement":"内联SVG","evidence":"<svg/>","assessment":"使用内联SVG容器"}],"issues":null}`,
		`{"label":"normal","reason":"完整","basis":[{"requirement":"内联SVG","evidence":"<svg/>","assessment":"使用内联SVG容器"}],"issues":[],"extra":true}`,
		`{"label":"normal","reason":"完整","basis":[{"requirement":"内联SVG","evidence":"<svg/>","assessment":"使用内联SVG容器"}],"issues":[]} {}`,
		`{"label":"suspicious","reason":"很差","basis":[{"requirement":"内联SVG","evidence":"<svg/>","assessment":"只有空容器"}],"issues":[{"severity":"major","scope":"motion","evidence":"<svg/>","impact":""}]}`,
	} {
		got := q.ParseReview(output, "使用内联SVG", `<svg/>`)
		if got.Status != "unknown" || got.Reason == "" {
			t.Fatalf("accepted invalid response: %+v", got)
		}
	}
}
