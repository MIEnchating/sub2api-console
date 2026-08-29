package accountops

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type testTarget struct {
	value configstore.TargetSettings
	calls atomic.Int32
}

func (target *testTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	target.calls.Add(1)
	return target.value, nil
}

type accountTaskObserver struct {
	updates chan taskstore.Task
}

func (observer *accountTaskObserver) Save(_ context.Context, task taskstore.Task) error {
	if task.Status == "succeeded" || task.Status == "failed" {
		observer.updates <- task
	}
	return nil
}

func waitAccountTask(t *testing.T, updates <-chan taskstore.Task) taskstore.Task {
	t.Helper()
	select {
	case task := <-updates:
		return task
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for account task")
		return taskstore.Task{}
	}
}

func TestManualAccountFieldsIgnoreAutomaticSchedulingWritebackVerification(t *testing.T) {
	repository, db, _ := accountRepository(t)
	if _, err := repository.UpdatePolicy(context.Background(), map[string]any{
		"advanced_policy": map[string]any{"writeback": map[string]any{"verification": true}},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	var written atomic.Bool
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			var body map[string]any
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil || body["rate_multiplier"] != json.Number("0.2") ||
				body["priority"] != json.Number("17") || body["load_factor"] != json.Number("3") ||
				body["concurrency"] != json.Number("25") {
				t.Fatalf("body=%#v err=%v", body, err)
			}
			written.Store(true)
			_, _ = io.WriteString(w, `{"success":true}`)
			return
		}
		reads.Add(1)
		name, multiplier, notes := "alpha", "0.1", "old"
		priority, loadFactor, concurrency := 10, "2", 1
		if written.Load() {
			name, multiplier, notes = "renamed", "0.2", "new"
			priority, loadFactor, concurrency = 17, "3", 25
		}
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"`+name+`","priority":`+strconv.Itoa(priority)+`,"load_factor":`+loadFactor+`,"concurrency":`+strconv.Itoa(concurrency)+`,"rate_multiplier":`+multiplier+`,"notes":"`+notes+`"}}`)
	}))
	defer server.Close()
	target := &testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}
	service := New(target, repository, nil)
	name, multiplier, notes := "renamed", "0.2", "new"
	priority, loadFactor, concurrency := int64(17), "3", int64(25)
	result, err := service.SyncFields(context.Background(), "41", FieldPatch{
		NamePresent: true, Name: &name, PriorityPresent: true, Priority: &priority,
		LoadFactorPresent: true, LoadFactor: &loadFactor, ConcurrencyPresent: true, Concurrency: &concurrency,
		MultiplierPresent: true, Multiplier: &multiplier,
		NotesPresent: true, Notes: &notes,
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result["remote_write"] != true || result["readback_confirmed"] != true || reads.Load() != 2 {
		t.Fatalf("result=%#v", result)
	}
	var storedName, storedLoadFactor, storedMultiplier, metadata string
	var storedPriority, storedConcurrency int64
	if err := db.QueryRow(`SELECT name,priority,load_factor,concurrency,multiplier,metadata_json FROM accounts WHERE id='41'`).Scan(&storedName, &storedPriority, &storedLoadFactor, &storedConcurrency, &storedMultiplier, &metadata); err != nil {
		t.Fatal(err)
	}
	if storedName != "renamed" || storedPriority != 17 || storedLoadFactor != "3" || storedConcurrency != 25 || storedMultiplier != "0.2" || !strings.Contains(metadata, `"notes":"new"`) {
		t.Fatalf("name=%q priority=%d load=%q concurrency=%d multiplier=%q metadata=%s", storedName, storedPriority, storedLoadFactor, storedConcurrency, storedMultiplier, metadata)
	}
}

func TestAccountFieldsRejectMismatchedReadbackBeforeLocalCommit(t *testing.T) {
	repository, db, _ := accountRepository(t)
	var reads, writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			writes.Add(1)
			_, _ = io.WriteString(w, `{"success":true}`)
			return
		}
		reads.Add(1)
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","priority":10,"load_factor":2,"concurrency":1,"rate_multiplier":0.1,"notes":"old"}}`)
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, nil)
	priority := int64(17)
	_, err := service.SyncFields(context.Background(), "41", FieldPatch{PriorityPresent: true, Priority: &priority}, "operator")
	if err == nil || !strings.Contains(err.Error(), "账号优先级读回不一致") {
		t.Fatalf("mismatched readback accepted: %v", err)
	}
	if reads.Load() != 2 || writes.Load() != 1 {
		t.Fatalf("reads=%d writes=%d", reads.Load(), writes.Load())
	}
	var storedPriority int64
	if err := db.QueryRow(`SELECT priority FROM accounts WHERE id='41'`).Scan(&storedPriority); err != nil {
		t.Fatal(err)
	}
	if storedPriority != 10 {
		t.Fatalf("unconfirmed priority committed locally: %d", storedPriority)
	}
}

func TestSyncAccountMultiplierRequiresMatchingReadbackBeforeLocalCommit(t *testing.T) {
	repository, db, _ := accountRepository(t)
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			_, _ = io.WriteString(w, `{"success":true}`)
			return
		}
		read := reads.Add(1)
		multiplier := "0.1"
		if read > 1 {
			multiplier = "0.16"
		}
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","rate_multiplier":`+multiplier+`}}`)
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, nil)

	_, err := service.SyncAccountMultiplier(context.Background(), "41", "0.15", "operator")
	if err == nil || !strings.Contains(err.Error(), "读回不一致") || reads.Load() != 2 {
		t.Fatalf("reads=%d err=%v", reads.Load(), err)
	}
	var multiplier string
	if err := db.QueryRow(`SELECT multiplier FROM accounts WHERE id='41'`).Scan(&multiplier); err != nil {
		t.Fatal(err)
	}
	if multiplier != "0.1" {
		t.Fatalf("unconfirmed multiplier committed locally: %s", multiplier)
	}
}

func TestSyncAccountMultiplierCommitsConfirmedReadback(t *testing.T) {
	repository, db, _ := accountRepository(t)
	if _, err := db.Exec(`INSERT INTO account_groups(account_id,group_name,group_id,group_rate)
		VALUES('41','codex','7','0.2')`); err != nil {
		t.Fatal(err)
	}
	var written atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			written.Store(true)
			_, _ = io.WriteString(w, `{"success":true}`)
			return
		}
		multiplier := "0.1"
		if written.Load() {
			multiplier = "0.15"
		}
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","rate_multiplier":`+multiplier+`}}`)
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, nil)

	result, err := service.SyncAccountMultiplier(context.Background(), "41", "0.15", "operator")
	if err != nil || result["readback_confirmed"] != true {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var multiplier, groupRate string
	if err := db.QueryRow(`SELECT a.multiplier,ag.group_rate FROM accounts a
		JOIN account_groups ag ON ag.account_id=a.id WHERE a.id='41'`).Scan(&multiplier, &groupRate); err != nil {
		t.Fatal(err)
	}
	if multiplier != "0.15" || groupRate != "0.15" {
		t.Fatalf("multiplier=%s group_rate=%s", multiplier, groupRate)
	}
}

func TestAccountControlTaskWritesSchedulableAndCommitsConfirmedPause(t *testing.T) {
	repository, db, _ := accountRepository(t)
	var paused atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/41/schedulable":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["schedulable"] != false {
				t.Fatalf("pause body=%#v err=%v", body, err)
			}
			paused.Store(true)
			_, _ = io.WriteString(w, `{"success":true}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41":
			_, _ = io.WriteString(w, fmt.Sprintf(`{"data":{"id":41,"name":"alpha","schedulable":%t}}`, !paused.Load()))
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	tasks := &accountTaskObserver{updates: make(chan taskstore.Task, 1)}
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, tasks)

	if _, err := service.EnqueueControl(context.Background(), "41", "pause", "operator"); err != nil {
		t.Fatal(err)
	}
	finished := waitAccountTask(t, tasks.updates)
	if finished.Status != "succeeded" || finished.Result["remote_write"] != true || finished.Result["readback_confirmed"] != true {
		t.Fatalf("control task=%#v", finished)
	}
	var schedulable, storedPaused bool
	if err := db.QueryRow(`SELECT schedulable,paused FROM accounts WHERE id='41'`).Scan(&schedulable, &storedPaused); err != nil {
		t.Fatal(err)
	}
	if schedulable || !storedPaused {
		t.Fatalf("schedulable=%v paused=%v", schedulable, storedPaused)
	}
	var operationType string
	var remoteConfirmed, readbackConfirmed bool
	if err := db.QueryRow(`SELECT operation_type,remote_confirmed,readback_confirmed FROM operation_audit
		WHERE object_id='41' ORDER BY source_id ASC LIMIT 1`).Scan(&operationType, &remoteConfirmed, &readbackConfirmed); err != nil {
		t.Fatal(err)
	}
	if operationType != "account.control" || !remoteConfirmed || !readbackConfirmed {
		t.Fatalf("operation=%s remote=%v readback=%v", operationType, remoteConfirmed, readbackConfirmed)
	}
}

func TestAccountControlTaskDoesNotCommitLocalPauseWhenRemoteWriteFails(t *testing.T) {
	repository, db, _ := accountRepository(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","schedulable":true}}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"scheduler unavailable"}}`)
	}))
	defer server.Close()
	tasks := &accountTaskObserver{updates: make(chan taskstore.Task, 1)}
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, tasks)

	if _, err := service.EnqueueControl(context.Background(), "41", "pause", "operator"); err != nil {
		t.Fatal(err)
	}
	finished := waitAccountTask(t, tasks.updates)
	if finished.Status != "failed" || finished.Result["remote_write"] != false {
		t.Fatalf("control task=%#v", finished)
	}
	var schedulable, storedPaused bool
	if err := db.QueryRow(`SELECT schedulable,paused FROM accounts WHERE id='41'`).Scan(&schedulable, &storedPaused); err != nil {
		t.Fatal(err)
	}
	if !schedulable || storedPaused {
		t.Fatalf("failed remote write changed local state: schedulable=%v paused=%v", schedulable, storedPaused)
	}
}

func TestAccountControlRejectsMismatchedReadbackWithoutCommittingLocalState(t *testing.T) {
	repository, db, _ := accountRepository(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"success":true}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","schedulable":true}}`)
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, nil)

	_, err := service.Control(context.Background(), "41", "pause", "operator")
	if err == nil || !strings.Contains(err.Error(), "读回不一致") {
		t.Fatalf("mismatched readback accepted: %v", err)
	}
	var schedulable, storedPaused bool
	if err := db.QueryRow(`SELECT schedulable,paused FROM accounts WHERE id='41'`).Scan(&schedulable, &storedPaused); err != nil {
		t.Fatal(err)
	}
	if !schedulable || storedPaused {
		t.Fatalf("mismatched readback changed local state: schedulable=%v paused=%v", schedulable, storedPaused)
	}
}

func TestAccountControlRecoversRuntimeAndSupportsLegacySchedulingEndpoint(t *testing.T) {
	repository, db, _ := accountRepository(t)
	if _, err := db.Exec(`UPDATE accounts SET schedulable=0,metadata_json=? WHERE id='41'`,
		`{"last_error":"rate limited","rate_limit_reset_at":"2099-01-01T00:00:00Z","notes":"keep"}`); err != nil {
		t.Fatal(err)
	}
	var recovered atomic.Bool
	var legacyWrite, clearError, recoverState atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/schedulable"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"message":"route not found"}}`)
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/admin/accounts/41":
			legacyWrite.Add(1)
			recovered.Store(true)
			_, _ = io.WriteString(w, `{"success":true}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41":
			_, _ = io.WriteString(w, fmt.Sprintf(`{"data":{"id":41,"name":"alpha","schedulable":%t}}`, recovered.Load()))
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/clear-error"):
			clearError.Add(1)
			_, _ = io.WriteString(w, `{"success":true}`)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/recover-state"):
			recoverState.Add(1)
			_, _ = io.WriteString(w, `{"success":true}`)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, nil)

	result, err := service.Control(context.Background(), "41", "recover", "operator")
	if err != nil || result["readback_confirmed"] != true {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if legacyWrite.Load() != 1 || clearError.Load() != 1 || recoverState.Load() != 1 {
		t.Fatalf("legacy=%d clear=%d recover=%d", legacyWrite.Load(), clearError.Load(), recoverState.Load())
	}
	var metadata string
	if err := db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='41'`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, "last_error") || strings.Contains(metadata, "rate_limit_reset_at") || !strings.Contains(metadata, `"notes":"keep"`) {
		t.Fatalf("metadata=%s", metadata)
	}
}

func TestAccountScopeControlStaysLocalAndDoesNotRequireManagementTarget(t *testing.T) {
	repository, _, _ := accountRepository(t)
	if _, err := repository.SetMode(context.Background(), "调度模式"); err != nil {
		t.Fatal(err)
	}
	target := &testTarget{}
	tasks := &accountTaskObserver{updates: make(chan taskstore.Task, 1)}
	service := New(target, repository, tasks)

	if _, err := service.EnqueueControl(context.Background(), "41", "exclude", "operator"); err != nil {
		t.Fatal(err)
	}
	finished := waitAccountTask(t, tasks.updates)
	if finished.Status != "succeeded" || finished.Result["remote_write"] != false {
		t.Fatalf("scope task=%#v", finished)
	}
	if target.calls.Load() != 0 {
		t.Fatalf("local scope control read management target %d times", target.calls.Load())
	}
	policy, err := repository.ControlPolicy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	scope := policy["scope"].(map[string]any)
	if values, ok := scope["excluded_account_ids"].([]any); !ok || len(values) != 1 || fmt.Sprint(values[0]) != "41" {
		t.Fatalf("excluded accounts=%#v", values)
	}
}

func TestManualPriorityTaskWritesSub2APIDefaultsAndCommitsAssignment(t *testing.T) {
	repository, db, _ := accountRepository(t)
	var written atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			var body map[string]any
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil || body["priority"] != json.Number("3") ||
				body["load_factor"] != json.Number("100") || body["concurrency"] != json.Number("100") {
				t.Fatalf("manual priority write body=%#v err=%v", body, err)
			}
			written.Store(true)
			_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","priority":3,"load_factor":1,"concurrency":3,"rate_multiplier":0.1}}`)
			return
		}
		if written.Load() {
			_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","priority":3,"load_factor":100,"concurrency":100,"rate_multiplier":0.1}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","priority":10,"load_factor":2,"concurrency":1,"rate_multiplier":0.1}}`)
	}))
	defer server.Close()
	tasks := &accountTaskObserver{updates: make(chan taskstore.Task, 1)}
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, tasks)
	if _, err := service.EnqueueManualPriority(context.Background(), "41", 3, "100", 100, "operator"); err != nil {
		t.Fatal(err)
	}
	finished := waitAccountTask(t, tasks.updates)
	if finished.Status != "succeeded" {
		t.Fatalf("manual priority task failed: %#v", finished)
	}
	var priority, concurrency, manualPriority int64
	var loadFactor string
	if err := db.QueryRow(`SELECT a.priority,a.load_factor,a.concurrency,m.priority
		FROM accounts a JOIN manual_priority_accounts m ON m.account_id=a.id WHERE a.id='41'`).
		Scan(&priority, &loadFactor, &concurrency, &manualPriority); err != nil {
		t.Fatal(err)
	}
	if priority != 3 || loadFactor != "100" || concurrency != 100 || manualPriority != 3 {
		t.Fatalf("priority=%d load=%s concurrency=%d manual=%d", priority, loadFactor, concurrency, manualPriority)
	}
}

func TestManualPriorityTaskRestoresPreviousSlotWhenRemoteWriteFails(t *testing.T) {
	repository, db, _ := accountRepository(t)
	if _, err := repository.AssignManualPriority(context.Background(), "41", 2, "100", 100, "operator"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"upstream unavailable"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","priority":2,"load_factor":1,"concurrency":3,"rate_multiplier":0.1}}`)
	}))
	defer server.Close()
	tasks := &accountTaskObserver{updates: make(chan taskstore.Task, 1)}
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, tasks)
	if _, err := service.EnqueueManualPriority(context.Background(), "41", 3, "100", 100, "operator"); err != nil {
		t.Fatal(err)
	}
	finished := waitAccountTask(t, tasks.updates)
	if finished.Status != "failed" {
		t.Fatalf("failed remote write was reported as success: %#v", finished)
	}
	var manualPriority int64
	if err := db.QueryRow(`SELECT priority FROM manual_priority_accounts WHERE account_id='41'`).Scan(&manualPriority); err != nil {
		t.Fatal(err)
	}
	if manualPriority != 2 {
		t.Fatalf("failed update lost previous manual slot: %d", manualPriority)
	}
}

func TestClearManualPriorityTaskRestoresRemoteBaselineBeforeLocalRelease(t *testing.T) {
	repository, db, _ := accountRepository(t)
	if _, err := repository.AssignManualPriority(context.Background(), "41", 3, "100", 100, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accounts SET priority=3,load_factor='100',concurrency=100 WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	var written atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodPut:
			var body map[string]any
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil || body["priority"] != json.Number("10") ||
				body["load_factor"] != json.Number("2") || body["concurrency"] != json.Number("3") {
				t.Fatalf("clear body=%#v err=%v", body, err)
			}
			written.Store(true)
			_, _ = io.WriteString(w, `{"success":true}`)
		case http.MethodGet:
			priority, loadFactor, concurrency := 3, 100, 100
			if written.Load() {
				priority, loadFactor, concurrency = 10, 2, 3
			}
			_, _ = io.WriteString(w, fmt.Sprintf(`{"data":{"id":41,"name":"alpha","schedulable":true,"priority":%d,"load_factor":%d,"concurrency":%d}}`, priority, loadFactor, concurrency))
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	tasks := &accountTaskObserver{updates: make(chan taskstore.Task, 1)}
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, tasks)

	if _, err := service.EnqueueClearManualPriority(context.Background(), "41", "operator"); err != nil {
		t.Fatal(err)
	}
	finished := waitAccountTask(t, tasks.updates)
	if finished.Status != "succeeded" || finished.Result["remote_write"] != true || finished.Result["readback_confirmed"] != true {
		t.Fatalf("clear task=%#v", finished)
	}
	var priority, concurrency int64
	var loadFactor string
	if err := db.QueryRow(`SELECT priority,load_factor,concurrency FROM accounts WHERE id='41'`).Scan(&priority, &loadFactor, &concurrency); err != nil {
		t.Fatal(err)
	}
	if priority != 10 || loadFactor != "2" || concurrency != 3 {
		t.Fatalf("priority=%d load=%s concurrency=%d", priority, loadFactor, concurrency)
	}
	var assignments int
	if err := db.QueryRow(`SELECT COUNT(*) FROM manual_priority_accounts WHERE account_id='41'`).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if assignments != 0 {
		t.Fatalf("manual assignment still present: %d", assignments)
	}
}

func TestClearManualPriorityTaskKeepsLocalAssignmentWhenReadbackMismatches(t *testing.T) {
	repository, db, _ := accountRepository(t)
	if _, err := repository.AssignManualPriority(context.Background(), "41", 3, "100", 100, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accounts SET priority=3,load_factor='100',concurrency=100 WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			_, _ = io.WriteString(w, `{"success":true}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","schedulable":true,"priority":3,"load_factor":100,"concurrency":100}}`)
	}))
	defer server.Close()
	tasks := &accountTaskObserver{updates: make(chan taskstore.Task, 1)}
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, tasks)

	if _, err := service.EnqueueClearManualPriority(context.Background(), "41", "operator"); err != nil {
		t.Fatal(err)
	}
	finished := waitAccountTask(t, tasks.updates)
	if finished.Status != "failed" || finished.Result["remote_write"] != true {
		t.Fatalf("clear task=%#v", finished)
	}
	var manualPriority int64
	if err := db.QueryRow(`SELECT priority FROM manual_priority_accounts WHERE account_id='41'`).Scan(&manualPriority); err != nil {
		t.Fatal(err)
	}
	if manualPriority != 3 {
		t.Fatalf("unconfirmed clear released local assignment: %d", manualPriority)
	}
}

func TestClearManualPriorityTaskRestoresNullableLoadFactor(t *testing.T) {
	repository, db, _ := accountRepository(t)
	if _, err := db.Exec(`UPDATE accounts SET load_factor=NULL WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.AssignManualPriority(context.Background(), "41", 3, "100", 100, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accounts SET priority=3,load_factor='100',concurrency=100 WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	var written atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			var body map[string]any
			decoder := json.NewDecoder(request.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&body); err != nil || body["load_factor"] != json.Number("0") {
				t.Fatalf("clear nullable load body=%#v err=%v", body, err)
			}
			written.Store(true)
			_, _ = io.WriteString(w, `{"success":true}`)
			return
		}
		if written.Load() {
			_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","schedulable":true,"priority":10,"concurrency":3}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":41,"name":"alpha","schedulable":true,"priority":3,"load_factor":100,"concurrency":100}}`)
	}))
	defer server.Close()
	tasks := &accountTaskObserver{updates: make(chan taskstore.Task, 1)}
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, tasks)

	if _, err := service.EnqueueClearManualPriority(context.Background(), "41", "operator"); err != nil {
		t.Fatal(err)
	}
	finished := waitAccountTask(t, tasks.updates)
	if finished.Status != "succeeded" || finished.Result["readback_confirmed"] != true {
		t.Fatalf("clear task=%#v", finished)
	}
	var loadFactor sql.NullString
	if err := db.QueryRow(`SELECT load_factor FROM accounts WHERE id='41'`).Scan(&loadFactor); err != nil {
		t.Fatal(err)
	}
	if loadFactor.Valid {
		t.Fatalf("nullable load factor restored as %q", loadFactor.String)
	}
}

func accountRepository(t *testing.T) (*business.Store, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "account-ops.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	if err := repository.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO accounts(
		id,name,schedulable,priority,load_factor,concurrency,multiplier,metadata_json,updated_at
	) VALUES('41','alpha',1,10,'2',3,'0.1','{"notes":"old"}','now')`); err != nil {
		t.Fatal(err)
	}
	return repository, db, path
}

func TestModelsPersistsNormalizedAccountModelCache(t *testing.T) {
	repository, db, _ := accountRepository(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/admin/accounts/41/models/sync-upstream" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"model-a"},{"id":"model-a"},{"id":"model-b"}]}`)
	}))
	defer server.Close()
	service := New(&testTarget{value: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 2}}, repository, nil)
	models, err := service.Models(context.Background(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != "model-a" || models[1] != "model-b" {
		t.Fatalf("models=%#v", models)
	}
	var metadata string
	if err := db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='41'`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadata, `"known_models":["model-a","model-b"]`) || !strings.Contains(metadata, `"notes":"old"`) {
		t.Fatalf("metadata=%s", metadata)
	}
}
