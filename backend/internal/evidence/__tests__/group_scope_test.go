package evidence_test

import (
	"database/sql"
	"slices"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
)

func TestEvidenceGroupScopeUsesStableIDsForPlanningAndCollection(t *testing.T) {
	for _, test := range []struct {
		name  string
		patch map[string]any
		want  []string
	}{
		{"selected ID does not include numeric group name", map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"managed_group_mode": "selected", "managed_group_ids": []any{"7"}}}}, []string{"41"}},
		{"excluded ID does not exclude numeric group name", map[string]any{"excluded_group_ids": []any{"7"}}, []string{"42"}},
		{"empty selected IDs include no groups", map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"managed_group_mode": "selected", "managed_group_ids": []any{}}}}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProbeBatchFixture(t, []string{"41", "42"}, nil)
			db, err := sql.Open("sqlite", fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`UPDATE account_groups SET group_id='7' WHERE account_id='41';
				UPDATE account_groups SET group_name='7',group_id='8' WHERE account_id='42'`); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.store.UpdatePolicy(t.Context(), test.patch, "test"); err != nil {
				t.Fatal(err)
			}
			fixture.policy, err = fixture.store.ControlPolicy(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			plan, err := fixture.service.Plan(t.Context(), fixture.policy, nil, nil, fixture.now)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(plan.ProbeAccountIDs, test.want) {
				t.Fatalf("automatic plan used a group name as an ID: got=%v want=%v", plan.ProbeAccountIDs, test.want)
			}
			_, err = fixture.service.Collect(t.Context(), fixture.policy, fixture.admin, evidence.Options{Now: fixture.now, ProbesAllowed: true})
			if err != nil {
				t.Fatal(err)
			}
			if actual := fixture.probeIDs(); !slices.Equal(actual, test.want) {
				t.Fatalf("automatic collection probed outside the selected IDs: got=%v want=%v", actual, test.want)
			}
		})
	}
}
