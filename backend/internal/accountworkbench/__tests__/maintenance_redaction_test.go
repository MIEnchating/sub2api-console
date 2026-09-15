package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMaintenanceAccountNameContainingCredentialsDoesNotExposeThemInTask(t *testing.T) {
	f, config := maintenanceFixture(t, nil)
	account := f.remote.accounts["101"]
	credentials := account["credentials"].(map[string]any)
	credentials["access_token"] = "isolated-private-access-value"
	account["name"] = "Account isolated-private-access-value test-admin-key"
	if _, err := f.service.CheckMaintenance(context.Background(), config.Revision, true); err != nil {
		t.Fatal(err)
	}
	terminal := f.await(t)
	raw, err := json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"isolated-private-access-value", "test-admin-key"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("maintenance task exposed a credential through the account name")
		}
	}
}
