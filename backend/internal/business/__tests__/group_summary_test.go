package business_test

import "testing"

func TestGroupSummaryAveragesOnlyScoredMembersAndKeepsMultipleMemberships(t *testing.T) {
	store, db := groupParticipationStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at) VALUES
 ('42','scored',1,'{}','now'),('43','unscored',1,'{}','now');
 INSERT INTO account_groups(account_id,group_name,group_id) VALUES
 ('42','codex','7'),('43','codex','7'),('41','7','8');
 INSERT INTO account_health_evaluations(account_id,group_name,health_score,sample_count,evaluated_at) VALUES
 ('41','codex',80.25,1,'now'),('42','codex',90.5,1,'now');`); err != nil {
		t.Fatal(err)
	}
	groups, err := store.Groups(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups=%+v", groups)
	}
	for _, group := range groups {
		wantCount, wantAverage := int64(2), 85.4
		if group.Name == "7" {
			wantCount, wantAverage = 1, 80.2
		}
		if group.ScoredAccounts != wantCount || group.AverageHealthScore == nil || *group.AverageHealthScore != wantAverage {
			t.Errorf("%s scored summary=%+v want count=%d average=%v", group.Name, group, wantCount, wantAverage)
		}
	}
}

func TestGroupSummaryKeepsExactMembershipNamesAndManagedAccountScope(t *testing.T) {
	store, db := groupParticipationStore(t)
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,updated_at) VALUES('Codex','9','now'),('empty','10','now');
 UPDATE accounts SET metadata_json='{"type":"apikey","platform":"openai"}' WHERE id='41';
 INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at) VALUES
 ('42','closed',0,'{"type":"apikey","platform":"openai"}','now'),
 ('43','unknown',NULL,'{"type":"apikey","platform":"openai"}','now'),
 ('44','outside',1,'{"type":"oauth","platform":"openai"}','now');
 INSERT INTO account_groups(account_id,group_name,group_id) VALUES
 ('41','Codex','9'),('41','7','8'),('42','codex',NULL),('43','codex','7'),('44','codex','7');`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"account_types": []any{"apikey"}, "platforms": []any{"openai"}}}}, "test"); err != nil {
		t.Fatal(err)
	}
	groups, err := store.Groups(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][4]int64{"codex": {3, 1, 1, 1}, "Codex": {1, 1, 0, 0}, "7": {1, 1, 0, 0}, "empty": {0, 0, 0, 0}}
	if len(groups) != len(want) {
		t.Fatalf("groups=%+v", groups)
	}
	for _, group := range groups {
		got := [4]int64{group.AccountCount, group.SchedulingOpen, group.SchedulingClosed, group.SchedulingUnknown}
		if got != want[group.Name] {
			t.Errorf("%s counts=%v want=%v", group.Name, got, want[group.Name])
		}
		if group.AverageHealthScore != nil || group.ScoredAccounts != 0 {
			t.Errorf("unscored accounts gained a score: %+v", group)
		}
	}
}
