package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func ptr(value int64) *int64 { return &value }

func TestPrecheckReasoningUsagePreservesProtocolCounts(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		want        *int64
	}{
		{"responses", `{"input_tokens":209,"output_tokens":1264,"output_tokens_details":{"reasoning_tokens":832}}`, ptr(832)},
		{"chat", `{"prompt_tokens":209,"completion_tokens":1264,"completion_tokens_details":{"reasoning_tokens":832}}`, ptr(832)},
		{"zero", `{"output_tokens_details":{"reasoning_tokens":0}}`, ptr(0)},
		{"missing", `{"output_tokens":10}`, nil},
		{"negative", `{"output_tokens_details":{"reasoning_tokens":-1}}`, nil},
		{"fraction", `{"output_tokens_details":{"reasoning_tokens":1.5}}`, nil},
		{"unsafe integer", `{"output_tokens_details":{"reasoning_tokens":9007199254740992}}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprintf(w, `{"output_text":"21","usage":%s}`, tc.usage)
			})
			input := request("1")
			input.Mode = "precheck"
			if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			raw, _ := json.Marshal(row.Usage)
			var usage struct {
				ReasoningTokens *int64 `json:"reasoning_tokens"`
			}
			_ = json.Unmarshal(raw, &usage)
			if (usage.ReasoningTokens == nil) != (tc.want == nil) || usage.ReasoningTokens != nil && *usage.ReasoningTokens != *tc.want {
				t.Fatalf("reasoning usage mismatch: %s", raw)
			}
		})
	}
}

func TestAnimationUsagePreservesMissingZeroAndProtocolTotals(t *testing.T) {
	for _, tc := range []struct {
		name, platform, body string
		input, output, total *int64
	}{
		{"missing remains unknown", "openai", fmt.Sprintf(`{"output_text":%q}`, fixtureSVG), nil, nil, nil},
		{"explicit zero remains zero", "openai", fmt.Sprintf(`{"output_text":%q,"usage":{"input_tokens":0,"output_tokens":0}}`, fixtureSVG), ptr(int64(0)), ptr(int64(0)), ptr(int64(0))},
		{"invalid usage remains unknown", "openai", fmt.Sprintf(`{"output_text":%q,"usage":{"input_tokens":-1,"output_tokens":1.5,"total_tokens":9007199254740992}}`, fixtureSVG), nil, nil, nil},
		{"partial usage does not invent total", "openai", fmt.Sprintf(`{"output_text":%q,"usage":{"output_tokens":20}}`, fixtureSVG), nil, ptr(int64(20)), nil},
		{"responses terminal usage", "openai", fmt.Sprintf("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":%q,\"usage\":{\"input_tokens\":11,\"output_tokens\":22,\"total_tokens\":33}}}\n\n", fixtureSVG), ptr(int64(11)), ptr(int64(22)), ptr(int64(33))},
		{"messages merge cumulative usage", "anthropic", fmt.Sprintf("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":%q}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":25}}\n\ndata: {\"type\":\"message_stop\"}\n\n", fixtureSVG), ptr(int64(10)), ptr(int64(25)), ptr(int64(35))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, tc.platform, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, tc.body) })
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			row := task.Result["animations"].([]modelcheck.AnimationResult)[0]
			if row.Status != "succeeded" || row.Usage == nil {
				t.Fatalf("generation failed: %+v", row)
			}
			assertAnimationUsage(t, row.Usage, tc.input, tc.output, tc.total)
		})
	}
}

func assertAnimationUsage(t *testing.T, usage *modelcheck.AnimationUsage, input, output, total *int64) {
	t.Helper()
	for _, field := range []struct {
		name      string
		got, want *int64
	}{{"input", usage.InputTokens, input}, {"output", usage.OutputTokens, output}, {"total", usage.TotalTokens, total}} {
		if (field.got == nil) != (field.want == nil) || (field.got != nil && *field.got != *field.want) {
			t.Errorf("%s usage mismatch: %+v", field.name, usage)
		}
	}
}

func TestOAuthAnimationPreservesHTMLUsageAndReportedModel(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Model     string
			Reasoning struct{ Effort string }
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "test-model" || body.Reasoning.Effort != "low" {
			t.Errorf("wrong OAuth settings: %+v", body)
		}
		value := fmt.Sprintf("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"reported-model\",\"output_text\":%q,\"usage\":{\"input_tokens\":10,\"output_tokens\":20,\"output_tokens_details\":{\"reasoning_tokens\":8}}}}\n\n", htmlAnimation)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(value))}, nil
	}))
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if row.Status != "succeeded" || row.HTML == "" || row.Source != htmlAnimation || row.ResponseModel != "reported-model" {
		t.Fatalf("lost OAuth artifact: %+v", row)
	}
	assertAnimationUsage(t, row.Usage, ptr(int64(10)), ptr(int64(20)), ptr(int64(30)))
	if row.Usage.ReasoningTokens == nil || *row.Usage.ReasoningTokens != 8 {
		t.Fatal("OAuth stream lost reasoning usage")
	}
}

func TestOAuthAnimationReportedModelNeverExposesExecutionCredentials(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		body := fmt.Sprintf(`{"status":"completed","model":%q,"output_text":%q}`, "reported-"+oauthFixtureToken+"-isolated-refresh-token", fixtureSVG)
		return oauthResponse(200, "application/json", body), nil
	}))
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	raw, _ := json.Marshal(task)
	if task.Status != "succeeded" || strings.Contains(string(raw), oauthFixtureToken) || strings.Contains(string(raw), "isolated-refresh-token") {
		t.Fatal("reported model leaked credentials or prevented safe display")
	}
}
