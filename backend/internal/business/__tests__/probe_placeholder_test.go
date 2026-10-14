package business_test

import (
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestProbePlaceholderMarkerSurvivesLaterFailureAndOlderSamples(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	for _, sample := range []business.ProbeSample{
		{AccountID: "41", GroupName: "a", Result: "通过", ObservedAt: "2026-10-11T12:00:00Z", RequestModel: "gpt-6-astra", IgnoredPlaceholder: true, IgnoredPlaceholderPreview: `"__-"`, IgnoredPlaceholderAttempt: 2},
		{AccountID: "41", GroupName: "a", Result: "失败", ObservedAt: "2026-10-11T13:00:00Z"},
		{AccountID: "41", GroupName: "b", Result: "失败", ObservedAt: "2026-10-11T11:00:00Z", IgnoredPlaceholder: true, IgnoredPlaceholderPreview: `"old"`},
	} {
		if _, err := store.PersistProbeSamples(t.Context(), []business.ProbeSample{sample}); err != nil {
			t.Fatal(err)
		}
	}
	accounts, err := store.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		raw, _ := json.Marshal(account)
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if account.ID == "41" {
			if fields["probe_placeholder_at"] != "2026-10-11T12:00:00.000000000Z" {
				t.Fatalf("missing durable latest marker: %s", raw)
			}
			placeholder, ok := fields["probe_placeholder"].(map[string]any)
			if !ok || placeholder["preview"] != `"__-"` || placeholder["group_name"] != "a" || placeholder["request_model"] != "gpt-6-astra" || placeholder["result"] != "通过" || placeholder["attempt"] != float64(2) {
				t.Fatalf("latest probe details missing or replaced: %s", raw)
			}
		} else if fields["probe_placeholder_at"] != nil {
			t.Fatalf("marker leaked to account %s", account.ID)
		}
	}
	detail, err := store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(detail)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	if fields["probe_placeholder_at"] != "2026-10-11T12:00:00.000000000Z" {
		t.Fatalf("detail lost marker: %s", raw)
	}
	if fields["probe_placeholder"] == nil {
		t.Fatalf("detail lost probe details: %s", raw)
	}
}

func TestDeletingMarkedAccountClearsMarkerBeforeIDReuse(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	if _, err := store.PersistProbeSamples(t.Context(), []business.ProbeSample{{AccountID: "41", GroupName: "a", Result: "失败", ObservedAt: "2026-10-11T12:00:00Z", IgnoredPlaceholder: true}}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAccountProjection(t.Context(), "41", business.AccountOperation{OperationID: "delete-placeholder", OperationType: "account.delete", State: "succeeded", Phase: "readback", Actor: "test", RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41"}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM operational_snapshots WHERE namespace='probe-placeholder' AND state_key='41'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("deleted marker remains: %d, %v", count, err)
	}
}

func TestLegacyProbePlaceholderIsReadableAndReplacedByNewObservation(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	if _, err := db.Exec(`INSERT INTO operational_snapshots(namespace,state_key,value_json,observed_at,updated_at) VALUES('probe-placeholder','41','{}','2026-10-11T12:00:00.000000000Z','2026-10-11T12:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	detail, err := store.Account(t.Context(), "41")
	if err != nil || detail == nil || detail.ProbePlaceholderAt == nil || detail.ProbePlaceholder != nil {
		t.Fatalf("legacy marker not preserved: %+v, %v", detail, err)
	}
	if _, err := store.PersistProbeSamples(t.Context(), []business.ProbeSample{{
		AccountID: "41", GroupName: "b", Result: "失败", ObservedAt: "2026-10-11T13:00:00Z",
		IgnoredPlaceholder: true, IgnoredPlaceholderPreview: `"\n"`, IgnoredPlaceholderAttempt: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	detail, err = store.Account(t.Context(), "41")
	if err != nil || detail == nil || detail.ProbePlaceholderAt == nil || *detail.ProbePlaceholderAt != "2026-10-11T13:00:00.000000000Z" || detail.ProbePlaceholder == nil || detail.ProbePlaceholder.Preview != `"\n"` || detail.ProbePlaceholder.GroupName != "b" {
		t.Fatalf("new observation did not replace legacy marker: %+v, %v", detail, err)
	}
}

func TestProbePlaceholderRecoveryUsesObservationOrder(t *testing.T) {
	abnormal := business.ProbeSample{AccountID: "41", GroupName: "a", Result: "通过", ObservedAt: "2026-10-11T12:00:00Z", IgnoredPlaceholder: true, IgnoredPlaceholderPreview: `"__-"`}
	clean := business.ProbeSample{AccountID: "41", GroupName: "b", Result: "通过", ObservedAt: "2026-10-11T13:00:00Z"}
	older := clean
	older.ObservedAt = "2026-10-11T11:00:00Z"
	newer := abnormal
	newer.ObservedAt = "2026-10-11T14:00:00Z"
	timeout := clean
	timeout.Result = "超时"
	skipped := clean
	skipped.Result = "跳过"
	for _, scenario := range []struct {
		name    string
		samples []business.ProbeSample
		marked  bool
	}{
		{"later clean success clears", []business.ProbeSample{abnormal, clean}, false},
		{"older success preserves", []business.ProbeSample{abnormal, older}, true},
		{"late old abnormal cannot restore", []business.ProbeSample{abnormal, clean, abnormal}, false},
		{"clean first blocks older abnormal", []business.ProbeSample{clean, abnormal}, false},
		{"new abnormal restores", []business.ProbeSample{abnormal, clean, newer}, true},
		{"timeout preserves", []business.ProbeSample{abnormal, timeout}, true},
		{"skipped preserves", []business.ProbeSample{abnormal, skipped}, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, db := concurrencyStore(t)
			seedCapacityAccounts(t, db)
			for _, sample := range scenario.samples {
				if _, err := store.PersistProbeSamples(t.Context(), []business.ProbeSample{sample}); err != nil {
					t.Fatal(err)
				}
			}
			detail, err := store.Account(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			if (detail.ProbePlaceholderAt != nil) != scenario.marked {
				t.Fatalf("marker=%v want marked=%v", detail.ProbePlaceholderAt, scenario.marked)
			}
			if !scenario.marked && detail.ProbePlaceholder != nil {
				t.Fatal("cleared marker retained details")
			}
			accounts, err := store.Accounts(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for _, account := range accounts {
				if account.ID == "41" && (account.ProbePlaceholderAt != nil) != scenario.marked {
					t.Fatal("list recovery disagrees with detail")
				}
			}
		})
	}
}

func TestCleanProbeClearsLegacyPlaceholder(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	if _, err := db.Exec(`INSERT INTO operational_snapshots(namespace,state_key,value_json,observed_at,updated_at) VALUES('probe-placeholder','41','{}','2026-10-11T12:00:00.000000000Z','2026-10-11T12:00:00.000000000Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistProbeSamples(t.Context(), []business.ProbeSample{{AccountID: "41", GroupName: "a", Result: "通过", ObservedAt: "2026-10-11T13:00:00Z"}}); err != nil {
		t.Fatal(err)
	}
	detail, err := store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.ProbePlaceholderAt != nil || detail.ProbePlaceholder != nil {
		t.Fatal("legacy marker remains after clean probe")
	}
}

func TestCleanProbePreservesAbnormalHistory(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	samples := []business.ProbeSample{
		{AccountID: "41", GroupName: "a", Result: "通过", ObservedAt: "2026-10-11T12:00:00Z", IgnoredPlaceholder: true, IgnoredPlaceholderPreview: `"__-"`},
		{AccountID: "41", GroupName: "a", Result: "通过", ObservedAt: "2026-10-11T13:00:00Z"},
	}
	if _, err := store.PersistProbeSamples(t.Context(), samples); err != nil {
		t.Fatal(err)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload_json FROM health_samples WHERE account_id='41' AND source='active-probe' ORDER BY observed_at LIMIT 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var evidence map[string]any
	if err := json.Unmarshal([]byte(payload), &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence["ignored_placeholder"] != true || evidence["ignored_placeholder_preview"] != `"__-"` {
		t.Fatalf("original evidence lost: %s", payload)
	}
}
