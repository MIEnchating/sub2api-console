package accountworkbench_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestAutomaticOAuthCheckpointReconnectsActiveSessionWithoutSnapshotOrNewAuthorization(t *testing.T) {
	f, factory := automaticCheckpointFixture(t)
	started := startAutomaticCheckpoint(t, f)
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 1 {
		t.Fatalf("active checkpoint = %+v, %v", list, err)
	}
	action := accountworkbench.OAuthCheckpointAction{Revision: list[0].Revision, Confirmed: true}
	reconnected, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", list[0].ID, action)
	if err != nil {
		t.Fatalf("active authorization could not reconnect without a worker snapshot: %v", err)
	}
	if reconnected.ID != started.ID || reconnected.TaskID != started.TaskID || reconnected.CheckpointID != started.CheckpointID || reconnected.ExpiresAt != started.ExpiresAt || reconnected.Status != "waiting" || reconnected.Image == "" {
		t.Fatalf("reconnect did not return the original live authorization: %+v", reconnected)
	}
	factory.mu.Lock()
	restores := factory.restores
	factory.mu.Unlock()
	if restores != 0 || f.exchanges.Load() != 0 {
		t.Fatal("reconnecting restored a browser or submitted an authorization code")
	}
	data, _ := json.Marshal(list[0])
	var metadata map[string]any
	_ = json.Unmarshal(data, &metadata)
	if metadata["active"] != true || !list[0].CanRestore {
		t.Fatalf("active checkpoint was not discoverable: %s", data)
	}
	for _, invalid := range []struct {
		name, owner string
		action      accountworkbench.OAuthCheckpointAction
	}{
		{"other owner", "other-owner", action},
		{"other scope", "checkpoint-owner", accountworkbench.OAuthCheckpointAction{Scope: accountworkbench.ScopeLocalExport, Revision: action.Revision, Confirmed: true}},
		{"stale record", "checkpoint-owner", accountworkbench.OAuthCheckpointAction{Revision: action.Revision + 1, Confirmed: true}},
		{"unconfirmed", "checkpoint-owner", accountworkbench.OAuthCheckpointAction{Revision: action.Revision}},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), invalid.owner, list[0].ID, invalid.action); err == nil {
				t.Fatal("reconnect accepted an invalid authorization binding")
			}
		})
	}
}

func TestAutomaticOAuthCheckpointWithoutSnapshotBecomesUnavailableAfterInterruption(t *testing.T) {
	f, _ := automaticCheckpointFixture(t)
	view := startAutomaticCheckpoint(t, f)
	if !f.runner.CancelTask(view.ID) {
		t.Fatal("could not interrupt the authorization")
	}
	f.phase(t, "checkpoint-review")
	select {
	case <-f.completed:
	case <-time.After(5 * time.Second):
		t.Fatal("interrupted authorization did not close")
	}
	list, err := f.service.OAuthCheckpoints(context.Background(), "checkpoint-owner")
	if err != nil || len(list) != 1 || list[0].Active || list[0].CanRestore {
		t.Fatalf("interrupted checkpoint without snapshot = %+v, %v", list, err)
	}
	if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", list[0].ID, accountworkbench.OAuthCheckpointAction{Revision: list[0].Revision, Confirmed: true}); err == nil {
		t.Fatal("interrupted authorization restored without a snapshot")
	}
}
