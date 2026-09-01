package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestEvaluateSelectsLowestProfitableCompatibleManagedGroup(t *testing.T) {
	config := Config{Enabled: true, ProfitMargin: 0.2, ExchangeGroupSets: [][]string{{"6", "7"}, {"8", "10"}}, IntervalSeconds: 120, WriteConcurrency: 4}
	catalog := business.PricingCatalog{
		Groups: []business.PricingGroup{
			{ID: "6", Name: "标准", Platform: "openai", RateMultiplier: testString("1")},
			{ID: "7", Name: "低价", Platform: "openai", RateMultiplier: testString("0.5")},
			{ID: "8", Name: "Claude", Platform: "anthropic", RateMultiplier: testString("1")},
			{ID: "10", Name: "复合", Platform: "composite", RateMultiplier: testString("1")},
		},
		Accounts: []business.PricingAccount{
			{ID: "41", Name: "高成本", Platform: "openai", Multiplier: testString("0.6"), GroupIDs: []string{"7", "9", "10"}, GroupsValid: true},
			{ID: "42", Name: "低成本", Platform: "openai", Multiplier: testString("0.4"), GroupIDs: []string{"6"}, GroupsValid: true},
			{ID: "43", Name: "未知成本", Platform: "openai", GroupIDs: []string{"7"}, GroupsValid: true},
		},
	}

	snapshot, err := evaluate(config, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Accounts != 3 || snapshot.Changes != 2 || snapshot.Skipped != 1 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if !reflect.DeepEqual(snapshot.Decisions[0].DesiredGroupIDs, []string{"6", "9", "10"}) {
		t.Fatalf("high-cost desired groups=%#v", snapshot.Decisions[0])
	}
	if !reflect.DeepEqual(snapshot.Decisions[1].DesiredGroupIDs, []string{"7"}) {
		t.Fatalf("low-cost desired groups=%#v", snapshot.Decisions[1])
	}
	if !snapshot.Decisions[2].Skipped || !reflect.DeepEqual(snapshot.Decisions[2].DesiredGroupIDs, []string{"7"}) {
		t.Fatalf("unknown-cost account was not preserved: %#v", snapshot.Decisions[2])
	}
	if snapshot.Groups[3].Available || snapshot.Groups[3].Reason == nil {
		t.Fatalf("composite group should remain unmanaged by allocation: %#v", snapshot.Groups[3])
	}
}

func TestEvaluateKeepsExchangeGroupSetsIsolated(t *testing.T) {
	config := Config{Enabled: true, ProfitMargin: 0.2, ExchangeGroupSets: [][]string{{"6", "7"}, {"8", "9"}}, IntervalSeconds: 120, WriteConcurrency: 4}
	catalog := business.PricingCatalog{
		Groups: []business.PricingGroup{
			{ID: "6", Name: "A 标准", Platform: "openai", RateMultiplier: testString("1")},
			{ID: "7", Name: "A 低价", Platform: "openai", RateMultiplier: testString("0.8")},
			{ID: "8", Name: "B 标准", Platform: "openai", RateMultiplier: testString("1")},
			{ID: "9", Name: "B 低价", Platform: "openai", RateMultiplier: testString("0.8")},
		},
		Accounts: []business.PricingAccount{{
			ID: "41", Name: "only-a", Platform: "openai", Multiplier: testString("0.4"), GroupIDs: []string{"6"}, GroupsValid: true,
		}},
	}

	snapshot, err := evaluate(config, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.Decisions[0].DesiredGroupIDs, []string{"7"}) {
		t.Fatalf("account crossed exchange-group boundary: %#v", snapshot.Decisions[0])
	}
}

func TestEvaluateUsesStableGroupIDToBreakEqualPriceTie(t *testing.T) {
	config := Config{Enabled: true, ProfitMargin: 0.2, ExchangeGroupSets: [][]string{{"7", "6"}}, IntervalSeconds: 120, WriteConcurrency: 4}
	catalog := business.PricingCatalog{
		Groups: []business.PricingGroup{
			{ID: "6", Name: "先选", Platform: "openai", RateMultiplier: testString("0.5")},
			{ID: "7", Name: "后选", Platform: "openai", RateMultiplier: testString("0.5")},
		},
		Accounts: []business.PricingAccount{{
			ID: "41", Name: "equal-price", Platform: "openai", Multiplier: testString("0.2"), GroupIDs: []string{"7"}, GroupsValid: true,
		}},
	}

	snapshot, err := evaluate(config, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.Decisions[0].DesiredGroupIDs, []string{"6"}) || !reflect.DeepEqual(snapshot.Decisions[0].EligibleGroups, []string{"先选"}) {
		t.Fatalf("equal-price decision=%#v", snapshot.Decisions[0])
	}
}

func TestEvaluateRepairsMultipleMembershipsWithinExchangeSet(t *testing.T) {
	config := Config{Enabled: true, ProfitMargin: 0.2, ExchangeGroupSets: [][]string{{"6", "7"}}, IntervalSeconds: 120, WriteConcurrency: 4}
	catalog := business.PricingCatalog{
		Groups: []business.PricingGroup{
			{ID: "6", Name: "标准", Platform: "openai", RateMultiplier: testString("1")},
			{ID: "7", Name: "低价", Platform: "openai", RateMultiplier: testString("0.5")},
		},
		Accounts: []business.PricingAccount{{
			ID: "41", Name: "duplicate", Platform: "openai", Multiplier: testString("0.4"), GroupIDs: []string{"6", "7", "9"}, GroupsValid: true,
		}},
	}

	snapshot, err := evaluate(config, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Decisions[0].Changed || !reflect.DeepEqual(snapshot.Decisions[0].DesiredGroupIDs, []string{"7", "9"}) {
		t.Fatalf("duplicate membership was not repaired: %#v", snapshot.Decisions[0])
	}
}

func TestValidatePriceGroupUniquenessRejectsMultipleTargetsInOneSet(t *testing.T) {
	err := validatePriceGroupUniqueness([]Decision{{
		AccountID: "41", DesiredGroupIDs: []string{"6", "7", "9"}, Changed: true,
	}}, Config{ExchangeGroupSets: [][]string{{"6", "7"}}})
	if err == nil || !strings.Contains(err.Error(), "多个目标分组") {
		t.Fatalf("duplicate target validation error=%v", err)
	}
}

func TestEvaluateNeverChangesManualPriorityAccountGroups(t *testing.T) {
	config := Config{Enabled: true, ProfitMargin: 0.2, ExchangeGroupSets: [][]string{{"6", "7"}}, IntervalSeconds: 120, WriteConcurrency: 4}
	catalog := business.PricingCatalog{
		Groups: []business.PricingGroup{
			{ID: "6", Name: "标准", Platform: "openai", RateMultiplier: testString("1")},
			{ID: "7", Name: "低价", Platform: "openai", RateMultiplier: testString("0.5")},
		},
		Accounts: []business.PricingAccount{{
			ID: "41", Name: "人工账号", Platform: "openai", Multiplier: testString("0.2"),
			GroupIDs: []string{"6"}, GroupsValid: true, ManualPriority: true,
		}},
	}

	snapshot, err := evaluate(config, catalog)
	if err != nil {
		t.Fatal(err)
	}
	decision := snapshot.Decisions[0]
	if !decision.Skipped || decision.Changed || !reflect.DeepEqual(decision.DesiredGroupIDs, []string{"6"}) ||
		decision.Reason == nil || !strings.Contains(*decision.Reason, "人工优先") {
		t.Fatalf("manual priority account was not protected: %#v", decision)
	}
}

func TestRestoreBackupUpdatesExistingAccountsAndSkipsManualPriority(t *testing.T) {
	accountGroups := map[string][]int64{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, "/api/v1/admin/accounts/") {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		accountID := strings.TrimPrefix(request.URL.Path, "/api/v1/admin/accounts/")
		if request.Method == http.MethodGet {
			writer.Header().Set("Content-Type", "application/json")
			groups := []int64{6}
			if saved, found := accountGroups[accountID]; found {
				groups = saved
			}
			_, _ = io.WriteString(writer, `{"success":true,"data":`+accountJSON(accountID, "0.4", groups)+`}`)
			return
		}
		if request.Method != http.MethodPut {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		var payload struct {
			GroupIDs []int64 `json:"group_ids"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		accountGroups[accountID] = payload.GroupIDs
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"success":true,"data":{"id":41}}`)
	}))
	defer server.Close()
	repository := &fakeRepository{
		catalog: business.PricingCatalog{
			Groups: []business.PricingGroup{{ID: "6"}, {ID: "7"}},
			Accounts: []business.PricingAccount{
				{ID: "41", GroupIDs: []string{"6"}, GroupsValid: true},
				{ID: "42", GroupIDs: []string{"6"}, GroupsValid: true, ManualPriority: true},
			},
		},
		backups: []business.PricingBackup{{ID: "backup-1", Accounts: []business.PricingBackupAccount{
			{AccountID: "41", GroupIDs: []string{"7"}}, {AccountID: "42", GroupIDs: []string{"7"}},
		}}},
	}
	service := New(repository, &fakeTargets{settings: configstore.TargetSettings{
		BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 5,
	}}, nil)
	result, err := service.RestoreBackupNow(context.Background(), "backup-1", "operator")
	if err != nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if result.Changed != 1 || result.Skipped != 1 || !reflect.DeepEqual(accountGroups["41"], []int64{7}) {
		t.Fatalf("result=%#v groups=%#v", result, accountGroups)
	}
	if _, changed := accountGroups["42"]; changed {
		t.Fatal("manual priority account was changed")
	}
}

func TestConfigDefaultsDisabledAndRequiresGroupBeforeEnabling(t *testing.T) {
	config, err := ConfigFromPolicy(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.ProfitMargin != 0.2 || config.IntervalSeconds != 120 || config.WriteConcurrency != 4 || len(config.ExchangeGroupSets) != 0 {
		t.Fatalf("defaults=%#v", config)
	}
	_, err = normalizeConfig(Config{Enabled: true, ProfitMargin: 0.2, IntervalSeconds: 120, WriteConcurrency: 4})
	if err == nil {
		t.Fatal("enabled config without managed groups was accepted")
	}
}

func TestLegacyManagedGroupsDisablePriceManagement(t *testing.T) {
	config, err := ConfigFromPolicy(map[string]any{"price_management": map[string]any{
		"enabled": true, "managed_group_ids": []any{"6", "7"}, "profit_margin": 0.2,
		"interval_seconds": int64(120), "write_concurrency": int64(4),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || len(config.ExchangeGroupSets) != 0 {
		t.Fatalf("legacy pricing config must require manual reconfiguration: %#v", config)
	}
}

func TestNormalizeConfigRejectsInvalidExchangeSets(t *testing.T) {
	tests := []struct {
		name string
		sets [][]string
	}{
		{name: "single group", sets: [][]string{{"6"}}},
		{name: "overlapping groups", sets: [][]string{{"6", "7"}, {"7", "8"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeConfig(Config{
				Enabled: true, ProfitMargin: 0.2, ExchangeGroupSets: test.sets,
				IntervalSeconds: 120, WriteConcurrency: 4,
			})
			if err == nil {
				t.Fatalf("invalid exchange sets were accepted: %#v", test.sets)
			}
		})
	}
}

func TestUpdateConfigRejectsMixedPlatformExchangeSet(t *testing.T) {
	repository := &fakeRepository{policy: map[string]any{}, catalog: business.PricingCatalog{Groups: []business.PricingGroup{
		{ID: "6", Name: "OpenAI", Platform: "openai", RateMultiplier: testString("1")},
		{ID: "7", Name: "Claude", Platform: "claude", RateMultiplier: testString("1")},
	}}}
	service := New(repository, nil, nil)

	_, err := service.UpdateConfig(context.Background(), Config{
		Enabled: true, ProfitMargin: 0.2, ExchangeGroupSets: [][]string{{"6", "7"}},
		IntervalSeconds: 120, WriteConcurrency: 4,
	}, "tester")
	if err == nil || !strings.Contains(err.Error(), "不能混合不同平台") {
		t.Fatalf("mixed-platform exchange set error=%v", err)
	}
}

func TestEvaluateKeepsAndExplainsAccountWithZeroCost(t *testing.T) {
	config := Config{Enabled: true, ProfitMargin: 0.2, ExchangeGroupSets: [][]string{{"6", "7"}}, IntervalSeconds: 120, WriteConcurrency: 4}
	catalog := business.PricingCatalog{
		Groups: []business.PricingGroup{{ID: "6", Name: "标准", Platform: "openai", RateMultiplier: testString("1")}},
		Accounts: []business.PricingAccount{{
			ID: "42", Name: "pool", Platform: "openai", Multiplier: testString("0"), GroupIDs: []string{"6"}, GroupsValid: true,
		}},
	}

	snapshot, err := evaluate(config, catalog)
	if err != nil {
		t.Fatal(err)
	}
	decision := snapshot.Decisions[0]
	if !decision.Skipped || decision.Changed || decision.CostMultiplier == nil || *decision.CostMultiplier != "0" || decision.Reason == nil || !strings.Contains(*decision.Reason, "必须大于 0") {
		t.Fatalf("zero-cost decision=%#v", decision)
	}
}

func TestSnapshotReadsConsoleCatalogWithoutManagementTarget(t *testing.T) {
	repository := &fakeRepository{
		policy: map[string]any{},
		catalog: business.PricingCatalog{
			Accounts: []business.PricingAccount{{
				ID: "41", Name: "local-account", Platform: "openai", Multiplier: testString("0.4"),
				GroupIDs: []string{"6"}, GroupsValid: true,
			}},
			Groups: []business.PricingGroup{{
				ID: "6", Name: "local-group", Platform: "openai", RateMultiplier: testString("1"),
			}},
		},
	}
	targets := fakeTargets{err: errors.New("预览不应读取管理平台连接")}
	service := New(repository, &targets, nil)

	snapshot, err := service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if targets.calls != 0 || snapshot.Accounts != 1 || snapshot.Decisions[0].AccountName != "local-account" {
		t.Fatalf("target calls=%d snapshot=%#v", targets.calls, snapshot)
	}
}

func TestApplyNowWritesOnlyChangesAndRefreshesLocalSnapshot(t *testing.T) {
	var mutex sync.Mutex
	accountGroups := map[string][]int64{"41": {7, 9}, "42": {6, 7}}
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		response.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/admin/accounts/41":
			putCount++
			var body struct {
				GroupIDs []int64 `json:"group_ids"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			accountGroups["41"] = append([]int64{}, body.GroupIDs...)
			_, _ = io.WriteString(response, `{"success":true,"data":{"id":41}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41":
			_, _ = io.WriteString(response, `{"success":true,"data":`+accountJSON("41", "0.6", accountGroups["41"])+`}`)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	repository := &fakeRepository{policy: map[string]any{"price_management": map[string]any{
		"enabled": true, "profit_margin": 0.2, "exchange_group_sets": []any{[]any{"6", "7"}}, "interval_seconds": int64(120), "write_concurrency": int64(2),
	}}, catalog: business.PricingCatalog{
		Groups: []business.PricingGroup{
			{ID: "6", Name: "标准", Platform: "openai", RateMultiplier: testString("1")},
			{ID: "7", Name: "低价", Platform: "openai", RateMultiplier: testString("0.5")},
		},
		Accounts: []business.PricingAccount{
			{ID: "41", Name: "account-41", Platform: "openai", Multiplier: testString("0.6"), GroupIDs: []string{"7", "9"}, GroupsValid: true},
			{ID: "42", Name: "account-42", Platform: "openai", Multiplier: testString("0.4"), GroupIDs: []string{"7"}, GroupsValid: true},
		},
	}}
	service := New(repository, &fakeTargets{settings: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "secret", TimeoutSeconds: 5}}, nil)

	result, err := service.ApplyNow(context.Background(), "tester")
	if err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if result.Changed != 1 || result.Unchanged != 1 || result.Failed != 0 || !result.RemoteWrite || putCount != 1 {
		t.Fatalf("result=%#v put=%d", result, putCount)
	}
	if !reflect.DeepEqual(accountGroups["41"], []int64{6, 9}) {
		t.Fatalf("account 41 groups=%#v", accountGroups["41"])
	}
	if repository.syncCalls != 1 || !reflect.DeepEqual(repository.syncedGroups, map[string][]string{"41": {"6", "9"}}) {
		t.Fatalf("local sync calls=%d groups=%#v", repository.syncCalls, repository.syncedGroups)
	}
}

func accountJSON(id, rate string, groups []int64) string {
	encoded, _ := json.Marshal(map[string]any{"id": json.Number(id), "name": "account-" + id, "platform": "openai", "rate_multiplier": json.Number(rate), "group_ids": groups})
	return string(encoded)
}

func testString(value string) *string { return &value }

type fakeRepository struct {
	policy       map[string]any
	catalog      business.PricingCatalog
	revenue      business.RevenueCatalog
	syncCalls    int
	syncedGroups map[string][]string
	backups      []business.PricingBackup
}

func (repository *fakeRepository) ControlPolicy(context.Context) (map[string]any, error) {
	return repository.policy, nil
}

func (repository *fakeRepository) UpdatePolicy(_ context.Context, patch map[string]any, _ string) (business.PolicySnapshot, error) {
	section := patch["advanced_policy"].(map[string]any)["price_management"]
	repository.policy["price_management"] = section
	return business.PolicySnapshot{}, nil
}

func (repository *fakeRepository) PricingCatalog(context.Context) (business.PricingCatalog, error) {
	return repository.catalog, nil
}

func (repository *fakeRepository) RevenueCatalog(context.Context) (business.RevenueCatalog, error) {
	return repository.revenue, nil
}

func (repository *fakeRepository) ValidateNewAPIQuotaUnit(context.Context, string, float64, time.Time, time.Time) error {
	return nil
}

func (repository *fakeRepository) SyncPricingAccountGroups(_ context.Context, groups map[string][]string, _ string) (business.PricingSyncResult, error) {
	repository.syncCalls++
	repository.syncedGroups = groups
	return business.PricingSyncResult{Accounts: len(groups)}, nil
}

func (repository *fakeRepository) CreatePricingBackup(_ context.Context, name, actor string) (business.PricingBackup, error) {
	backup := business.PricingBackup{ID: "backup-1", Name: name, Actor: actor, AccountCount: len(repository.catalog.Accounts)}
	repository.backups = append(repository.backups, backup)
	return backup, nil
}

func (repository *fakeRepository) PricingBackups(context.Context) ([]business.PricingBackup, error) {
	return repository.backups, nil
}

func (repository *fakeRepository) PricingBackup(_ context.Context, id string) (business.PricingBackup, error) {
	for _, backup := range repository.backups {
		if backup.ID == id {
			return backup, nil
		}
	}
	return business.PricingBackup{}, errors.New("missing backup")
}

type fakeTargets struct {
	settings configstore.TargetSettings
	auth     map[string]configstore.AuthRecord
	err      error
	calls    int
}

func (targets *fakeTargets) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	targets.calls++
	return targets.settings, targets.err
}

func (targets *fakeTargets) AuthRecord(_ context.Context, host string) (*configstore.AuthRecord, error) {
	record, found := targets.auth[configstore.CanonicalHost(host)]
	if !found {
		return nil, nil
	}
	return &record, nil
}
