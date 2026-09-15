package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func completedPrivateQueue(t *testing.T, f *batchFixture) configstore.WorkbenchQueue {
	t.Helper()
	view := startRecoverableBatch(t, f, "owner@example.com----private-queue-password")
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	digest := sha256.Sum256([]byte("owner"))
	record, err := f.private.WorkbenchQueue(context.Background(), hex.EncodeToString(digest[:]), queueTargetHash(t, f.importFixture), view.ID)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestOAuthQueueTamperedResultIdentityCannotResumeOrStartAnotherLogin(t *testing.T) {
	for _, boundary := range []string{"email", "workspace", "duplicate", "missing", "unknown-status"} {
		t.Run(boundary, func(t *testing.T) {
			f := newBatchFixture(t, nil)
			record := completedPrivateQueue(t, f)
			var payload map[string]any
			if err := json.Unmarshal(record.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			rows := payload["view"].(map[string]any)["items"].([]any)
			row := rows[0].(map[string]any)
			switch boundary {
			case "email":
				row["email"] = "different@example.test"
			case "workspace":
				row["workspace_id"] = "different-workspace"
			case "duplicate":
				payload["results"] = append(payload["results"].([]any), payload["results"].([]any)[0])
			case "missing":
				payload["results"] = []any{}
			case "unknown-status":
				row["status"] = "__proto__"
			}
			record.Payload, _ = json.Marshal(payload)
			saved, err := f.private.SaveWorkbenchQueue(context.Background(), record)
			if err != nil {
				t.Fatal(err)
			}
			service := resumedQueueService(t, f)
			if _, err := service.ResumeOAuthQueue(context.Background(), "owner", record.ID, accountworkbench.QueueRecoveryAction{Revision: saved.Revision, Confirmed: true}); err == nil {
				t.Fatal("inconsistent restored identity became available")
			}
			f.browsers.mu.Lock()
			defer f.browsers.mu.Unlock()
			if f.browsers.opened != 1 {
				t.Fatal("invalid recovery launched another browser")
			}
		})
	}
}
