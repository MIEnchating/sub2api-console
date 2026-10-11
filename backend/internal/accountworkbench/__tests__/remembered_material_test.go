package accountworkbench_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestRememberedLoginKeepsCorrectedPasswordAfterExchangeFailureAndSessionCleanup(t *testing.T) {
	f := importedRememberedFixture(t, true)
	protocol := &protocolFixture{t: t}
	passwords := []string{}
	failExchange := true
	signedRunInputWithProtocol(t, f.service, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/accounts/password/verify" {
			return protocol.response(req)
		}
		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, err
		}
		passwords = append(passwords, body.Password)
		if body.Password != "corrected-password" {
			response := upstreamResponse(map[string]any{"error": map[string]any{"code": "invalid_password"}})
			response.StatusCode = http.StatusBadRequest
			return response, nil
		}
		return upstreamResponse(map[string]any{"continue_url": "https://chatgpt.com/api/auth/callback/openai"}), nil
	}, func(*http.Request) error {
		if failExchange {
			return errors.New("isolated token exchange failure")
		}
		return nil
	})
	start := func() accountworkbench.Run {
		t.Helper()
		saved, err := f.service.RememberedAccounts(t.Context(), f.owner)
		if err != nil || len(saved) != 1 {
			t.Fatalf("saved source missing: %v %+v", err, saved)
		}
		preview, err := f.service.PreviewRememberedLogin(t.Context(), f.owner, saved[0].ID, saved[0].Revision)
		if err != nil {
			t.Fatal(err)
		}
		run, err := f.service.Start(t.Context(), f.owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
		if err != nil {
			t.Fatal(err)
		}
		return run
	}
	run := start()
	run = waitProtocolPrompt(t, f.service, f.owner, run.ID, "")
	if err := f.service.SubmitLoginInput(t.Context(), f.owner, run.ID, run.Items[0].ID, accountworkbench.LoginInput{PromptID: run.Items[0].LoginPrompt.ID, Value: "corrected-password"}); err != nil {
		t.Fatal(err)
	}
	awaitRun(t, f.runner)
	if err := f.store.RevokeSession(t.Context(), f.token); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.owner, _ = previewOwner(t, f.store)
	failExchange = false
	run = start()
	awaitRun(t, f.runner)
	result, err := f.service.Run(t.Context(), f.owner, run.ID)
	if err != nil || result.Status != "completed" || !slices.Equal(passwords, []string{"test-password", "corrected-password", "corrected-password"}) {
		t.Fatalf("corrected password was lost: %v status=%s password attempts=%d", err, result.Status, len(passwords))
	}
}
