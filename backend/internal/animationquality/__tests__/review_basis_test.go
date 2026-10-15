package animationquality_test

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
)

func TestReviewGeneratesGroundedReasonForNormalSample(t *testing.T) {
	output := `{"label":"normal","reason":"车轮旋转有明确实现，未发现主要缺陷。","basis":[{"requirement":"车轮转动","evidence":"wheel.style.transform = rotation","assessment":"每帧更新车轮旋转角度，支持主要转动要求。"}],"issues":[]}`
	got := q.ParseReview(output, "展示车轮转动", `wheel.style.transform = rotation`)
	if got.Status != "normal" || !strings.Contains(got.Explanation, "车轮转动") || !strings.Contains(got.Explanation, "每帧更新车轮旋转角度") || !strings.Contains(got.Explanation, "未运行或观看动画") {
		t.Fatalf("missing generated reason: %+v", got)
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), `"basis"`) || !strings.Contains(string(raw), "wheel.style.transform = rotation") {
		t.Fatalf("evidence not retained: %s", raw)
	}
}

func TestGeneratedReasonAtFieldLimitsRemainsCollectable(t *testing.T) {
	items := make([]q.ReviewBasis, 6)
	for i := range items {
		items[i] = q.ReviewBasis{Requirement: strings.Repeat("题", 80), Evidence: "source", Assessment: strings.Repeat("据", 140)}
	}
	raw, _ := json.Marshal(map[string]any{"label": "normal", "reason": strings.Repeat("理", 500), "basis": items, "issues": []any{}})
	got := q.ParseReview(string(raw), strings.Repeat("题", 80), "source")
	if got.Status != "normal" || utf8.RuneCountInString(got.Explanation) > 2000 || len(got.Basis) != 6 {
		t.Fatalf("reason exceeds sample limit or evidence lost: %+v", got)
	}
}

func TestReviewRejectsTooManyEvidenceItems(t *testing.T) {
	items := make([]q.ReviewBasis, 7)
	for i := range items {
		items[i] = q.ReviewBasis{Requirement: "车轮转动", Evidence: "wheel.rotate()", Assessment: "车轮有旋转实现"}
	}
	raw, _ := json.Marshal(map[string]any{"label": "normal", "reason": "结论", "basis": items, "issues": []any{}})
	if rejected := q.ParseReview(string(raw), "车轮转动", "wheel.rotate()"); rejected.Status != "unknown" || rejected.Reason != "review_invalid_response" {
		t.Fatalf("accepted oversized evidence list: %+v", rejected)
	}
}

func TestNormalReviewWithoutEvidenceCannotBecomeTrainingLabel(t *testing.T) {
	got := q.ParseReview(`{"label":"normal","reason":"很好","issues":[]}`, "展示车轮转动", `<svg/>`)
	if got.Status != "unknown" || got.Reason == "" {
		t.Fatalf("accepted unsupported reason: %+v", got)
	}
}

func TestReviewRejectsUngroundedBasis(t *testing.T) {
	for _, tc := range []struct{ name, requirement, evidence, assessment string }{
		{"invented requirement", "人物挥手", "wheel.rotate()", "有旋转实现"},
		{"invented source", "车轮转动", "wheel.stop()", "车轮静止"},
		{"empty assessment", "车轮转动", "wheel.rotate()", " "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{"label": "normal", "reason": "车轮有旋转实现", "basis": []map[string]string{{"requirement": tc.requirement, "evidence": tc.evidence, "assessment": tc.assessment}}, "issues": []any{}})
			got := q.ParseReview(string(raw), "车轮转动", "wheel.rotate()")
			if got.Status != "unknown" || got.Reason == "" || got.Explanation != "" || len(got.Basis) != 0 {
				t.Fatalf("invalid evidence became a usable reason: %+v", got)
			}
		})
	}
}

func TestUnknownReviewExplainsUncertaintyWithoutFabricatingEvidence(t *testing.T) {
	got := q.ParseReview(`{"label":"unknown","reason":"源码缺少主要动作实现，无法可靠判断。","basis":[],"issues":[]}`, "车轮转动", "")
	if got.Status != "unknown" || got.Reason != "" || !strings.Contains(got.Explanation, "无法可靠判断") || len(got.Basis) != 0 {
		t.Fatalf("%+v", got)
	}
}
