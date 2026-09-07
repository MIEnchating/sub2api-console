package modelcheck

import (
	"context"
	"errors"
	"testing"
)

type cancelledBundleSender struct{}

func (cancelledBundleSender) Send(context.Context, string, string, string, int) (string, string, error) {
	return "", "", context.Canceled
}

func TestClaudeCheckCancelledBeforeRequestsDoesNotReportSuccessfulRequests(t *testing.T) {
	profiles, err := loadClaudeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	var model string
	for name := range profiles {
		model = name
		break
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := runClaudeCheck(ctx, cancelledBundleSender{}, profiles, targetRequest{Model: model, Rounds: 1})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err == nil {
		requests := result["requests"].(map[string]any)
		if requests["successful"] != 0 || result["verdict"] != "ERROR" {
			t.Fatalf("unexecuted requests were reported successful: %#v", result)
		}
	}
}

func TestModelCheckAnswerWithMultipleArraysIsRejected(t *testing.T) {
	if answer := parseJSONArray(`["A"] ["B"]`); answer != nil {
		t.Fatalf("ambiguous answer accepted: %#v", answer)
	}
}
