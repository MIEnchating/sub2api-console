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

func TestTerminalContinuityRunsAsIndependentAccountTest(t *testing.T) {
	for _, tc := range []struct {
		name, answer, verdict string
	}{
		{"normal continuation", `{"tool":"exec_command","command":"git status --short"}`, "normal"},
		{"false permission denial", "当前会话没有可调用的 exec、git 或文件系统工具入口。", "suspected"},
		{"screenshot permission denial", "当前会话确实没有可用的终端、文件系统或 GitHub 查询工具入口；无法实际读取仓库。", "suspected"},
		{"unclear continuation", "我会继续检查。", "inconclusive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Input string `json:"input"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if !strings.Contains(body.Input, "exec_command") || !strings.Contains(body.Input, "对话上下文") {
					t.Errorf("unexpected prompt: %q", body.Input)
				}
				_, _ = fmt.Fprintf(w, `{"output_text":%q,"model":"returned-model"}`, tc.answer)
			})
			input := modelcheck.TerminalContinuityRequest{Targets: []modelcheck.AnimationTarget{{AccountID: "1", Model: "gpt-6-astra"}}, TimeoutSeconds: 5}
			if _, err := f.service.EnqueueTerminalContinuity(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			task := finished(t, f)
			rows := task.Result["checks"].([]modelcheck.TerminalContinuityResult)
			if task.Operation != "account-terminal-continuity" || len(rows) != 1 || rows[0].Verdict != tc.verdict || rows[0].Response != tc.answer || rows[0].ResponseModel != "returned-model" {
				t.Fatalf("task = %#v", task)
			}
			if task.Result["remote_write"] != false {
				t.Fatal("terminal continuity test must not write remote account state")
			}
			statuses, err := f.service.AccountStatuses(context.Background())
			if err != nil || len(statuses) != 0 {
				t.Fatal("terminal continuity test changed behavior status")
			}
		})
	}
}

func TestTerminalContinuityRejectsInvalidScopeAndTracksSeparateHistory(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"output_text":"{\"tool\":\"exec_command\",\"command\":\"git status --short\"}"}`)
	})
	for _, input := range []modelcheck.TerminalContinuityRequest{
		{},
		{Targets: []modelcheck.AnimationTarget{{AccountID: "0", Model: "model"}}, TimeoutSeconds: 5},
		{Targets: []modelcheck.AnimationTarget{{AccountID: "1", Model: ""}}, TimeoutSeconds: 5},
	} {
		if _, err := f.service.EnqueueTerminalContinuity(context.Background(), input); err == nil {
			t.Fatalf("invalid input accepted: %#v", input)
		}
	}
	if _, err := f.service.EnqueueTerminalContinuity(context.Background(), modelcheck.TerminalContinuityRequest{Targets: []modelcheck.AnimationTarget{{AccountID: "1", Model: "model"}}, TimeoutSeconds: 5}); err != nil {
		t.Fatal(err)
	}
	_ = finished(t, f)
	history, err := f.service.TerminalContinuityHistory(context.Background())
	if err != nil || len(history) != 1 || history[0].Skill != "sub2api-terminal-continuity" {
		t.Fatalf("history = %#v err = %v", history, err)
	}
	animations, err := f.service.AnimationHistory(context.Background())
	if err != nil || len(animations) != 0 {
		t.Fatalf("terminal task leaked into animation history: %#v err=%v", animations, err)
	}
}
