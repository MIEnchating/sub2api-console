package configstore_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func executionClockFixture(t *testing.T, start time.Time) (*configstore.Store, *sql.DB, string, func(time.Duration)) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private-execution.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var clock atomic.Int64
	clock.Store(start.UnixNano())
	store.UseWorkbenchExecutionClock(func() time.Time { return time.Unix(0, clock.Load()).UTC() })
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return store, db, path, func(delta time.Duration) { clock.Add(int64(delta)) }
}

func executionFixtureRecord(id string) configstore.WorkbenchExecution {
	return configstore.WorkbenchExecution{ID: id, TargetURL: "https://isolated.invalid", TargetFingerprint: "target-fingerprint", Items: []configstore.WorkbenchExecutionItem{{Index: 0, Name: "Private account", Email: "private@example.com", Credentials: json.RawMessage(`{"access_token":"private-retry-token"}`), Phase: "prepared", Status: "failed"}}}
}

func savedExecution(t *testing.T, store *configstore.Store, input configstore.WorkbenchExecution) configstore.WorkbenchExecution {
	t.Helper()
	if err := store.SaveWorkbenchExecution(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	record, err := store.WorkbenchExecution(context.Background(), input.ID)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func claimExecutionChild(t *testing.T, store *configstore.Store, parent configstore.WorkbenchExecution, id string) configstore.WorkbenchExecution {
	t.Helper()
	child := executionFixtureRecord(id)
	child.SourceID = parent.ID
	child = savedExecution(t, store, child)
	parent.Items[0].RetryTaskID = id
	parent.Items[0].Credentials = nil
	if err := store.SaveWorkbenchExecution(context.Background(), parent); err != nil {
		t.Fatal(err)
	}
	return child
}

func executionRowCount(t *testing.T, db *sql.DB, key string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM settings WHERE key=?`, key).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestWorkbenchExecutionFirstSaveOwnsFixedLifetimeAndUpdatesCannotExtendIt(t *testing.T) {
	start := time.Now().UTC()
	store, db, _, advance := executionClockFixture(t, start)
	input := executionFixtureRecord("fixed-lifetime")
	input.CreatedAt = start.AddDate(1, 0, 0).Format(time.RFC3339Nano)
	input.ExpiresAt = start.AddDate(2, 0, 0).Format(time.RFC3339Nano)
	record := savedExecution(t, store, input)
	created, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if err != nil || !created.Equal(start) {
		t.Fatalf("store creation time = %s, %v", record.CreatedAt, err)
	}
	expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil || expires.Sub(created) != 24*time.Hour {
		t.Fatal("execution was not assigned a fixed 24 hour lifetime")
	}
	advance(23 * time.Hour)
	record.CreatedAt = start.Add(time.Hour).Format(time.RFC3339Nano)
	record.ExpiresAt = start.Add(72 * time.Hour).Format(time.RFC3339Nano)
	record = savedExecution(t, store, record)
	actual, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil || !actual.Equal(expires) {
		t.Fatal("CAS update extended private credential lifetime")
	}
	advance(time.Hour)
	expired, err := store.WorkbenchExecution(context.Background(), record.ID)
	if !errors.Is(err, configstore.ErrWorkbenchExecution) || len(expired.Items) != 0 {
		t.Fatal("expired read returned private execution data")
	}
	if executionRowCount(t, db, "account_workbench.execution."+record.ID) != 0 {
		t.Fatal("expired read retained credential storage")
	}
	if err := store.SaveWorkbenchExecution(context.Background(), record); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatalf("stale CAS resurrected an expired record: %v", err)
	}
}

func TestWorkbenchExecutionExpiredWriteClearsCredentialBeforeRejectingUpdate(t *testing.T) {
	store, db, _, advance := executionClockFixture(t, time.Now().UTC())
	record := savedExecution(t, store, executionFixtureRecord("expired-write"))
	advance(24 * time.Hour)
	if err := store.SaveWorkbenchExecution(context.Background(), record); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatalf("expired write = %v", err)
	}
	if executionRowCount(t, db, "account_workbench.execution."+record.ID) != 0 {
		t.Fatal("expired write left private credentials in storage")
	}
}

func TestWorkbenchExecutionChildRemainsRetryableAfterExpiredParentIsPurged(t *testing.T) {
	store, db, _, advance := executionClockFixture(t, time.Now().UTC())
	parent := savedExecution(t, store, executionFixtureRecord("parent"))
	advance(23 * time.Hour)
	child := claimExecutionChild(t, store, parent, "child")
	advance(2 * time.Hour)
	if err := store.PurgeExpiredWorkbenchExecutions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executionRowCount(t, db, "account_workbench.execution.parent") != 0 {
		t.Fatal("expired parent credentials survived purge")
	}
	if err := store.ValidateWorkbenchExecutionClaim(context.Background(), child); err != nil {
		t.Fatalf("live child lost its parent authorization: %v", err)
	}
	var receipt string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key='account_workbench.execution_claims.parent'`).Scan(&receipt); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-retry-token", "private@example.com", "Private account", "credentials", "snapshot", "target_url"} {
		if strings.Contains(receipt, secret) {
			t.Fatalf("claim receipt retained private field %s", secret)
		}
	}
	advance(22 * time.Hour)
	if err := store.PurgeExpiredWorkbenchExecutions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executionRowCount(t, db, "account_workbench.execution.child") != 0 || executionRowCount(t, db, "account_workbench.execution_claims.parent") != 0 {
		t.Fatal("expired child or claim receipt survived retention")
	}
}

func TestWorkbenchExecutionRestartPurgesExpiredParentWhileKeepingLiveChildClaim(t *testing.T) {
	store, db, path, advance := executionClockFixture(t, time.Now().UTC().Add(-30*time.Hour))
	parent := savedExecution(t, store, executionFixtureRecord("parent"))
	advance(12 * time.Hour)
	child := claimExecutionChild(t, store, parent, "child")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if executionRowCount(t, db, "account_workbench.execution.parent") != 0 {
		t.Fatal("restart retained expired parent credentials")
	}
	if err := reopened.ValidateWorkbenchExecutionClaim(context.Background(), child); err != nil {
		t.Fatalf("restart lost a live child claim: %v", err)
	}
}

func TestWorkbenchExecutionGrandchildOutlivesBothAncestorsWithoutKeepingTheirCredentials(t *testing.T) {
	store, db, _, advance := executionClockFixture(t, time.Now().UTC())
	parent := savedExecution(t, store, executionFixtureRecord("parent"))
	advance(23 * time.Hour)
	child := claimExecutionChild(t, store, parent, "child")
	advance(23 * time.Hour)
	grandchild := claimExecutionChild(t, store, child, "grandchild")
	advance(2 * time.Hour)
	if err := store.PurgeExpiredWorkbenchExecutions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executionRowCount(t, db, "account_workbench.execution.parent") != 0 || executionRowCount(t, db, "account_workbench.execution.child") != 0 || executionRowCount(t, db, "account_workbench.execution_claims.parent") != 0 {
		t.Fatal("expired ancestors remained after successor generation")
	}
	if err := store.ValidateWorkbenchExecutionClaim(context.Background(), grandchild); err != nil {
		t.Fatalf("live grandchild lost its claim: %v", err)
	}
}

func TestWorkbenchExecutionExpiredSourceCannotCreateFreshPrivateRetryRecord(t *testing.T) {
	store, db, _, advance := executionClockFixture(t, time.Now().UTC())
	parent := savedExecution(t, store, executionFixtureRecord("parent"))
	advance(24 * time.Hour)
	child := executionFixtureRecord("child")
	child.SourceID = parent.ID
	if err := store.SaveWorkbenchExecution(context.Background(), child); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatalf("expired source created child: %v", err)
	}
	if executionRowCount(t, db, "account_workbench.execution.child") != 0 || executionRowCount(t, db, "account_workbench.execution.parent") != 0 {
		t.Fatal("expired source credentials were preserved or copied")
	}
}

func TestWorkbenchExecutionFailedSourceWriteRollsBackClaimReceipt(t *testing.T) {
	store, db, _, _ := executionClockFixture(t, time.Now().UTC())
	parent := savedExecution(t, store, executionFixtureRecord("parent"))
	child := executionFixtureRecord("child")
	child.SourceID = parent.ID
	child = savedExecution(t, store, child)
	if _, err := db.Exec(`CREATE TRIGGER fail_execution_claim BEFORE UPDATE ON settings WHEN NEW.key='account_workbench.execution.parent' BEGIN SELECT RAISE(FAIL,'isolated claim failure'); END`); err != nil {
		t.Fatal(err)
	}
	parent.Items[0].RetryTaskID = child.ID
	if err := store.SaveWorkbenchExecution(context.Background(), parent); err == nil {
		t.Fatal("injected source write failure was ignored")
	}
	if executionRowCount(t, db, "account_workbench.execution_claims.parent") != 0 {
		t.Fatal("failed source CAS left an authorized claim receipt")
	}
	if err := store.ValidateWorkbenchExecutionClaim(context.Background(), child); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatalf("uncommitted child became authorized: %v", err)
	}
}

func TestWorkbenchExecutionUnclaimedChildCannotCreateAnIndependentSuccessor(t *testing.T) {
	store, db, _, _ := executionClockFixture(t, time.Now().UTC())
	parent := savedExecution(t, store, executionFixtureRecord("parent"))
	child := executionFixtureRecord("unclaimed-child")
	child.SourceID = parent.ID
	child = savedExecution(t, store, child)
	grandchild := executionFixtureRecord("grandchild")
	grandchild.SourceID = child.ID
	if err := store.SaveWorkbenchExecution(context.Background(), grandchild); !errors.Is(err, configstore.ErrWorkbenchExecution) {
		t.Fatalf("unclaimed child created independent successor: %v", err)
	}
	if executionRowCount(t, db, "account_workbench.execution.grandchild") != 0 {
		t.Fatal("unclaimed execution copied credentials into a successor")
	}
}
