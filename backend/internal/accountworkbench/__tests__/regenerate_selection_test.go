package accountworkbench_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestRegenerationRejectsInvalidSourceChoicesAndAccountIDs(t *testing.T) {
	f, _ := exportFixture(t)
	for _, input := range []accountworkbench.RegenerationInput{
		{},
		{AccountIDs: []string{"0"}},
		{AccountIDs: []string{"01"}},
		{AccountIDs: []string{"101", "101"}},
		{AccountIDs: []string{"101"}, Indexes: []int{0}},
		{SourceTaskID: "../private"},
		{SourceTaskID: strings.Repeat("a", 32), AccountIDs: []string{"101"}},
		{SourceTaskID: strings.Repeat("a", 32), Indexes: []int{0, 0}},
		{SourceTaskID: strings.Repeat("a", 32), Indexes: []int{-1}},
		{SourceTaskID: strings.Repeat("a", 32), Indexes: []int{500}},
	} {
		if _, err := f.service.PreviewRegeneration(context.Background(), "regenerate-owner", input); err == nil {
			t.Fatalf("invalid regeneration selection accepted = %+v", input)
		}
	}
}

func TestRegenerationRejectsAccountsWithoutRefreshTokenOrCompleteStableIdentity(t *testing.T) {
	for _, missing := range []string{"refresh_token", "chatgpt_user_id", "chatgpt_account_id", "masked-token", "platform"} {
		t.Run(missing, func(t *testing.T) {
			f, _ := exportFixture(t)
			credentials := f.remote.accounts["101"]["credentials"].(map[string]any)
			switch missing {
			case "masked-token":
				credentials["refresh_token"] = "rt_***"
			case "platform":
				f.remote.accounts["101"]["platform"] = "anthropic"
			default:
				delete(credentials, missing)
			}
			if _, err := f.service.PreviewRegeneration(context.Background(), "regenerate-owner", accountworkbench.RegenerationInput{AccountIDs: []string{"101"}}); err == nil {
				t.Fatal("account without verified regeneration identity was accepted")
			}
		})
	}
}

func TestRegenerationUsesCompletedImportStableIDsAndCurrentCredentials(t *testing.T) {
	f, _ := exportFixture(t)
	f.remote.accounts = map[string]map[string]any{}
	view := f.preview(t, importedJSON, false)
	source := f.start(t, view)
	f.await(t)
	record, err := f.private.WorkbenchExecution(context.Background(), source.ID)
	if err != nil || len(record.Items) != 1 || record.Items[0].AccountID == "" {
		t.Fatalf("completed import scope = %+v, %v", record, err)
	}
	id := record.Items[0].AccountID
	f.remote.accounts[id]["credentials"].(map[string]any)["refresh_token"] = "rt_current_online_private"
	var refreshes atomic.Int32
	regenerationTransport(f, func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input["refresh_token"] != "rt_current_online_private" {
			t.Error("import source reused old saved input credentials")
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"regenerated-access-private","refresh_token":"rt_regenerated_private","chatgpt_user_id":"user-1","chatgpt_account_id":"workspace-1"}}`))
	})
	preview := previewRegeneration(t, f, "current-owner", accountworkbench.RegenerationInput{SourceTaskID: source.ID, Indexes: []int{0}})
	if preview.Items[0].AccountID != id || preview.Items[0].UserID != "user-1" || refreshes.Load() != 0 {
		t.Fatalf("completed import selection = %+v", preview)
	}
	if task := executeRegeneration(t, f, "current-owner", preview); task.Status != "succeeded" || refreshes.Load() != 1 {
		t.Fatalf("completed import regeneration = %+v, refreshes=%d", task, refreshes.Load())
	}
	f.remote.accounts[id]["credentials"].(map[string]any)["chatgpt_user_id"] = "other-user"
	if _, err := f.service.PreviewRegeneration(context.Background(), "current-owner", accountworkbench.RegenerationInput{SourceTaskID: source.ID}); err == nil {
		t.Fatal("changed official identity reused completed import scope")
	}
}

func TestRegenerationRejectsDuplicateIdentityAcrossStableAccountIDs(t *testing.T) {
	f, _ := exportFixture(t)
	f.remote.accounts["102"]["credentials"] = f.remote.accounts["101"]["credentials"]
	if _, err := f.service.PreviewRegeneration(context.Background(), "regenerate-owner", accountworkbench.RegenerationInput{AccountIDs: []string{"101", "102"}}); err == nil {
		t.Fatal("duplicate official identity would refresh the same token twice")
	}
}
