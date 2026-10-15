package upstreamconfig_test

import (
	"slices"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
)

func TestUpdateToKnownAliasReservesCanonicalVaultEntryForManualLogin(t *testing.T) {
	fixture := newAddressFixture(t, "https://"+addressSourceHost)
	fixture.addAlias(t, true)
	verifier := &vaultReservationVerifier{repository: fixture.repository}
	service := upstreamconfig.New(fixture.repository, fixture.private, verifier)
	username, password := "operator@example.test", "isolated-password"
	fixture.input.Host, fixture.input.BaseURL, fixture.input.AuthMode = "", addressAliasURL, "sub2api_manual_login"
	fixture.input.Username, fixture.input.Password, fixture.input.SaveToVault = &username, &password, true
	fixture.input.Entry, fixture.input.Present = nil, map[string]bool{"username": true, "password": true}

	if _, err := service.Update(t.Context(), addressSourceHost, fixture.input, "test"); err != nil {
		t.Fatal(err)
	}
	if verifier.concurrent {
		t.Fatal("saving the alias address allowed a competing editor to acquire the canonical login entry")
	}
	stored, err := fixture.private.VaultEntry(t.Context(), verifier.entryID)
	if err != nil || stored == nil || stored.Username == nil || *stored.Username != username || !slices.Contains(stored.Hosts, addressSourceHost) {
		t.Fatalf("manual login was not saved in its reserved canonical entry: err=%v", err)
	}
	aliasEntry, err := fixture.private.VaultEntry(t.Context(), addressAliasHost)
	if err != nil || aliasEntry != nil {
		t.Fatalf("saving the canonical login unexpectedly created an alias vault entry: err=%v", err)
	}
}
