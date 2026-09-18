package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestOAuthCombinedScheduleRestoresAndKeepsPrecheckAndAnimationResults(t *testing.T) {
	f, private := oauthAccountFixture(t, oauthAnimationAccount)
	if len(f.service.AnimationSchedules()) != 0 {
		t.Fatal("OAuth automatic detection must default off")
	}
	_, err := f.service.SaveAnimationSchedule(context.Background(), modelcheck.AnimationSchedule{
		AccountID: "1", Enabled: true, Model: "gpt-6-astra", Mode: "both", PrecheckQuestions: []string{"candy"}, IntervalMinutes: 10, TimeoutSeconds: 5,
	}, "isolated-test")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := modelcheck.New(f.tasks, private, &oauthProfileCatalog{catalog: f.catalog}, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	restarted.UseTaskRunner(f.runner)
	var requests []string
	restarted.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Input []struct {
				Content string `json:"content"`
			} `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		answer := "29"
		if strings.Contains(body.Input[0].Content, "SVG") {
			answer = fixtureSVG
			requests = append(requests, "animation")
		} else {
			requests = append(requests, "precheck")
		}
		return oauthResponse(200, "application/json", fmt.Sprintf(`{"status":"completed","output_text":%q}`, answer)), nil
	}))
	view := restarted.AnimationSchedules()[0]
	if view.Mode != "both" || len(view.PrecheckQuestions) != 1 {
		t.Fatalf("restored schedule: %#v", view)
	}
	next, err := time.Parse(time.RFC3339Nano, view.NextAt)
	if err != nil {
		t.Fatal(err)
	}
	restarted.RunDueAnimations(context.Background(), next)
	task := finished(t, f)
	rows := task.Result["animations"].([]modelcheck.AnimationResult)
	if task.Status != "succeeded" || len(rows) != 2 || rows[0].Precheck.Verdict != "not_passed" || rows[1].SVG == "" || strings.Join(requests, ",") != "precheck,animation" {
		t.Fatalf("combined OAuth schedule result: %#v", rows)
	}
	if rows[0].RequestID == rows[1].RequestID {
		t.Fatal("combined phases reused request IDs")
	}
}
