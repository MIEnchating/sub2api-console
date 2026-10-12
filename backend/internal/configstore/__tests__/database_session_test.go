package configstore_test

import (
	"testing"
	"time"
)

func TestDatabaseContractConcurrentInitializationHasOneWinner(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			start, results := make(chan struct{}), make(chan error, 4)
			for range 4 {
				go func() { <-start; results <- store.InitializeLocalExport(t.Context(), "operator", "initial-password") }()
			}
			close(start)
			succeeded := 0
			for range 4 {
				if <-results == nil {
					succeeded++
				}
			}
			if succeeded != 1 {
				t.Fatalf("initialization winners: %d", succeeded)
			}
			valid, err := store.Authenticate(t.Context(), "operator", "initial-password")
			if err != nil || !valid {
				t.Fatalf("winning credentials unavailable: %v", err)
			}
		})
	}
}

func TestDatabaseContractCredentialRotationRevokesOtherSessions(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
			token, err := store.InitializeSession(t.Context(), "operator", "initial-password", "", "", true, time.Hour, now)
			if err != nil {
				t.Fatal(err)
			}
			other, err := store.CreateAuthenticatedSession(t.Context(), "operator", "initial-password", time.Hour, now)
			if err != nil {
				t.Fatal(err)
			}
			password := "replacement-password"
			_, saved, err := store.UpdateSessionCredentials(t.Context(), token, "initial-password", "renamed", &password, time.Hour, now)
			if err != nil || saved != token {
				t.Fatalf("credential rotation: %v", err)
			}
			user, err := store.SessionUser(t.Context(), other, now)
			if err != nil || user != nil {
				t.Fatalf("old session survived: %v", err)
			}
			user, err = store.SessionUser(t.Context(), token, now)
			if err != nil || user == nil || *user != "renamed" {
				t.Fatalf("current session lost: %v", err)
			}
			if _, err := store.CreateAuthenticatedSession(t.Context(), "operator", "initial-password", time.Hour, now); err == nil {
				t.Fatal("old password accepted")
			}
		})
	}
}
