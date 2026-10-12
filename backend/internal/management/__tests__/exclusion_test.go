package management_test

import (
	"encoding/json"
	"testing"
)

func TestExcludedAccountsAreAbsentFromMaintenanceTaskScope(t *testing.T) {
	store, _, tasks, service := maintenanceDiscoveryFixture(t)
	if err := store.SetUpstreamExcluded(t.Context(), "selected", true, "test"); err != nil {
		t.Fatal(err)
	}
	task, err := service.EnqueueAccountBaseURLValidation(t.Context(), []string{"2", "30"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := tasks.Get(t.Context(), task.ID)
	if err != nil || saved.Result["requested"] != json.Number("1") {
		t.Fatalf("scope: %+v %v", saved, err)
	}
}
