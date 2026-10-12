package configstore_test

import (
	"fmt"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestDatabaseContractConcurrentPartialVaultEditsRetainBothFields(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			for index := range 8 {
				entry := fmt.Sprintf("concurrent-vault-%d", index)
				initial := "initial"
				if err := store.SaveVaultEntry(t.Context(), configstore.VaultEntry{Entry: entry, Username: &initial, Password: &initial}, nil); err != nil {
					t.Fatal(err)
				}
				start, results := make(chan struct{}), make(chan error, 2)
				username, password := "new-user", "new-password"
				go func() {
					<-start
					results <- store.SaveVaultEntry(t.Context(), configstore.VaultEntry{Entry: entry, Username: &username}, map[string]bool{"username": true})
				}()
				go func() {
					<-start
					results <- store.SaveVaultEntry(t.Context(), configstore.VaultEntry{Entry: entry, Password: &password}, map[string]bool{"password": true})
				}()
				close(start)
				for range 2 {
					if err := <-results; err != nil {
						t.Fatal(err)
					}
				}
				got, err := store.VaultEntry(t.Context(), entry)
				if err != nil || got == nil || got.Username == nil || got.Password == nil || *got.Username != username || *got.Password != password {
					t.Fatal("concurrent partial vault edit lost a field")
				}
			}
		})
	}
}
