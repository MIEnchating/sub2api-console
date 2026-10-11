package accountworkbench_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestRememberedLoginDeletedDuringAuthorizationPersistsTerminalResult(t *testing.T) {
	f := importedRememberedFixture(t, true)
	saved, err := f.service.RememberedAccounts(t.Context(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	protocol := &protocolFixture{t: t}
	signedRunInputWithProtocol(t, f.service, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/accounts/password/verify" {
			if err := f.service.DeleteRememberedAccount(req.Context(), f.owner, saved[0].ID, saved[0].Revision); err != nil {
				return nil, err
			}
		}
		return protocol.response(req)
	}, func(*http.Request) error { return nil })
	preview, err := f.service.PreviewRememberedLogin(t.Context(), f.owner, saved[0].ID, saved[0].Revision)
	if err != nil {
		t.Fatal(err)
	}
	writes := *f.writes
	run, err := f.service.Start(t.Context(), f.owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, f.runner)
	result, err := f.service.Run(t.Context(), f.owner, run.ID)
	if err != nil || result.Status != "interrupted" || *f.writes != writes {
		t.Fatalf("deleted source left a live task or wrote upstream: %v %+v", err, result)
	}
	remaining, err := f.service.RememberedAccounts(t.Context(), f.owner)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("deleted login source was recreated: %v %+v", err, remaining)
	}
}

func TestRememberedLoginUsesNewSessionAndCurrentAccountConfiguration(t *testing.T) {
	f := importedRememberedFixture(t, true)
	if err := f.store.RevokeSession(t.Context(), f.token); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	owner, _ := previewOwner(t, f.store)
	saved, err := f.service.RememberedAccounts(t.Context(), owner)
	if err != nil || len(saved) != 1 || !saved[0].LoginAvailable {
		t.Fatalf("login source unavailable: %v %+v", err, saved)
	}
	f.account["name"] = "当前名称"
	f.account["concurrency"] = json.Number("8")
	f.account["status"] = "error"
	preview, err := f.service.PreviewRememberedLogin(t.Context(), owner, saved[0].ID, saved[0].Revision)
	if err != nil || preview.Template == nil || preview.Template.Config.Concurrency != 8 || preview.Items[0].Name != "当前名称" {
		t.Fatalf("wrong login preview: %v %+v", err, preview)
	}
	run, err := f.service.Start(t.Context(), owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, f.runner)
	result, err := f.service.Run(t.Context(), owner, run.ID)
	if err != nil || result.Status != "completed" || result.Items[0].AccountID != "41" || f.account["name"] != "当前名称" || f.account["concurrency"] != json.Number("8") {
		t.Fatalf("bound relogin failed: %v %+v", err, result)
	}
}

func TestRememberedLoginRejectsDeletedSourceBeforeStart(t *testing.T) {
	f := importedRememberedFixture(t, true)
	saved, err := f.service.RememberedAccounts(t.Context(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewRememberedLogin(t.Context(), f.owner, saved[0].ID, saved[0].Revision)
	if err != nil {
		t.Fatal(err)
	}
	writes := *f.writes
	if err := f.service.DeleteRememberedAccount(t.Context(), f.owner, saved[0].ID, saved[0].Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Start(t.Context(), f.owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision}); err == nil {
		t.Fatal("deleted source started a login")
	}
	if *f.writes != writes {
		t.Fatal("deleted source wrote to upstream")
	}
}

func TestRememberedLoginRejectsAccountChangeAfterPreview(t *testing.T) {
	f := importedRememberedFixture(t, true)
	saved, err := f.service.RememberedAccounts(t.Context(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewRememberedLogin(t.Context(), f.owner, saved[0].ID, saved[0].Revision)
	if err != nil {
		t.Fatal(err)
	}
	writes := *f.writes
	f.account["priority"] = json.Number("20")
	run, err := f.service.Start(t.Context(), f.owner, accountworkbench.RunConfirmation{ID: preview.ID, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	awaitRun(t, f.runner)
	result, err := f.service.Run(t.Context(), f.owner, run.ID)
	if err != nil || result.Items[0].Status != "failed" || *f.writes != writes {
		t.Fatalf("changed account overwritten: %v %+v", err, result)
	}
}

func TestRememberedLoginRejectsChangedIdentityAndJSONOnlyInput(t *testing.T) {
	for _, login := range []bool{false, true} {
		f := importedRememberedFixture(t, login)
		saved, err := f.service.RememberedAccounts(t.Context(), f.owner)
		if err != nil {
			t.Fatal(err)
		}
		f.account["credentials"].(map[string]any)["chatgpt_account_id"] = "other-workspace"
		if _, err := f.service.PreviewRememberedLogin(t.Context(), f.owner, saved[0].ID, saved[0].Revision); err == nil {
			t.Fatal("unavailable or mismatched login source accepted")
		}
	}
}
