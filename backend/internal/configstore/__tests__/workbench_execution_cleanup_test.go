package configstore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestWorkbenchExecutionCleanupProcessesMultiplePagesWithoutDeletingLiveRecords(t *testing.T) {
	store, db, _, advance := executionClockFixture(t, time.Now().UTC())
	for index := range 70 {
		savedExecution(t, store, executionFixtureRecord(fmt.Sprintf("expired-%03d", index)))
	}
	advance(25 * time.Hour)
	live := savedExecution(t, store, executionFixtureRecord("live"))
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES('unrelated.private','retained-secret')`); err != nil {
		t.Fatal(err)
	}
	if err := store.PurgeExpiredWorkbenchExecutions(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM settings WHERE key GLOB 'account_workbench.execution.expired-*'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expired rows after paged cleanup = %d", count)
	}
	if _, err := store.WorkbenchExecution(context.Background(), live.ID); err != nil {
		t.Fatal("paged cleanup deleted an unexpired execution")
	}
	if executionRowCount(t, db, "unrelated.private") != 1 {
		t.Fatal("execution cleanup touched an unrelated private setting")
	}
}

func TestWorkbenchExecutionCleanupSerializesWithConcurrentExecutionUpdates(t *testing.T) {
	store, _, _, advance := executionClockFixture(t, time.Now().UTC())
	for index := range 66 {
		savedExecution(t, store, executionFixtureRecord(fmt.Sprintf("expired-%03d", index)))
	}
	advance(25 * time.Hour)
	records := make([]configstore.WorkbenchExecution, 12)
	for index := range records {
		records[index] = savedExecution(t, store, executionFixtureRecord(fmt.Sprintf("live-%03d", index)))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	failures := make(chan error, len(records)+2)
	var workers sync.WaitGroup
	for _, record := range records {
		workers.Go(func() {
			<-start
			record.Items[0].Status = "review"
			failures <- store.SaveWorkbenchExecution(ctx, record)
		})
	}
	for range 2 {
		workers.Go(func() { <-start; failures <- store.PurgeExpiredWorkbenchExecutions(ctx) })
	}
	close(start)
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatalf("concurrent cleanup or update failed: %v", err)
		}
	}
	for _, expected := range records {
		actual, err := store.WorkbenchExecution(ctx, expected.ID)
		if err != nil || actual.Revision != expected.Revision+1 || actual.ExpiresAt != expected.ExpiresAt || actual.Items[0].Status != "review" {
			t.Fatalf("cleanup lost a concurrent execution update: %v", err)
		}
	}
}

func TestWorkbenchExecutionCleanupPrunesExpiredSiblingClaimWithoutBlockingLiveSibling(t *testing.T) {
	store, db, _, advance := executionClockFixture(t, time.Now().UTC())
	parent := executionFixtureRecord("parent")
	secondItem := parent.Items[0]
	secondItem.Index = 1
	parent.Items = append(parent.Items, secondItem)
	parent = savedExecution(t, store, parent)
	advance(time.Hour)
	first := executionFixtureRecord("first-child")
	first.SourceID = parent.ID
	first = savedExecution(t, store, first)
	parent.Items[0].RetryTaskID, parent.Items[0].Credentials = first.ID, nil
	parent = savedExecution(t, store, parent)
	advance(time.Hour)
	second := executionFixtureRecord("second-child")
	second.SourceID, second.Items[0].Index = parent.ID, 1
	second = savedExecution(t, store, second)
	parent.Items[1].RetryTaskID, parent.Items[1].Credentials = second.ID, nil
	savedExecution(t, store, parent)
	advance(23 * time.Hour)
	if err := store.PurgeExpiredWorkbenchExecutions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateWorkbenchExecutionClaim(context.Background(), second); err != nil {
		t.Fatalf("live sibling lost claim after earlier sibling expired: %v", err)
	}
	var receipt string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key='account_workbench.execution_claims.parent'`).Scan(&receipt); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(receipt, first.ID) || !strings.Contains(receipt, second.ID) {
		t.Fatal("cleanup did not prune only the expired sibling claim")
	}
}

func TestWorkbenchExecutionMissingLifetimeCannotRetainLegacyCredentials(t *testing.T) {
	store, db, _, _ := executionClockFixture(t, time.Now().UTC())
	legacy := executionFixtureRecord("without-expiry")
	legacy.Revision = 1
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, "account_workbench.execution."+legacy.ID, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WorkbenchExecution(context.Background(), legacy.ID); err == nil {
		t.Fatal("execution without a verifiable expiry returned credentials")
	}
	if executionRowCount(t, db, "account_workbench.execution."+legacy.ID) != 0 {
		t.Fatal("execution without lifetime retained private credentials")
	}
}
