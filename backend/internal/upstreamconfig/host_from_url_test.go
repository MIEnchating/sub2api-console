package upstreamconfig

import (
	"context"
	"errors"
	"testing"
)

func TestUpdateDerivesHostFromAddressAndKeepsUpstreamIdentity(t *testing.T) {
	for _, test := range []struct {
		name, address, host string
	}{
		{"domain-and-path", "https://NEW.example.test/admin", "new.example.test"},
		{"http-and-port", "http://new.example.test:8080/admin", "new.example.test:8080"},
		{"ipv6", "https://[::1]:8443/admin", "[::1]:8443"},
		{"same-host", "https://old.example.test/admin", "old.example.test"},
	} {
		t.Run(test.name, func(t *testing.T) {
			private, repository := openStores(t)
			service := New(repository, private, &passVerifier{})
			ctx := context.Background()
			name, token := "测试上游", "test-token"
			input := Input{
				Host: "old.example.test", Name: &name, BaseURL: "https://old.example.test",
				AccountBaseURL: "https://models.example.test/v1", UpstreamType: "sub2api",
				AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &token,
				Present: map[string]bool{"access_token": true, "refresh_token": true},
			}
			original, err := service.Create(ctx, input, "tester")
			if err != nil {
				t.Fatal(err)
			}

			input.Host, input.BaseURL = "", test.address
			updated, err := service.Update(ctx, original.Host, input, "tester")
			if err != nil {
				t.Fatal(err)
			}
			if updated.Host != test.host || updated.BaseURL != test.address || updated.UpstreamID != original.UpstreamID || updated.AccountBaseURL != original.AccountBaseURL {
				t.Fatalf("address update changed identity or failed to derive Host: %#v", updated)
			}
			stored, err := private.AuthRecord(ctx, test.host)
			if err != nil || stored == nil || stored.Host != test.host || stored.BaseURL != test.address {
				t.Fatalf("authentication address was not updated: record=%#v err=%v", stored, err)
			}
			if test.host != original.Host {
				oldRecord, err := private.AuthRecord(ctx, original.Host)
				if err != nil || oldRecord != nil {
					t.Fatalf("old authentication record remains: %#v err=%v", oldRecord, err)
				}
			}
		})
	}
}

func TestUpdateRejectsInvalidOrConflictingAddressWithoutChangingConfiguration(t *testing.T) {
	for _, address := range []string{"existing.example.test", "ftp://existing.example.test", "https://user:pass@existing.example.test", "https://existing.example.test?key=value", "https://existing.example.test"} {
		t.Run(address, func(t *testing.T) {
			private, repository := openStores(t)
			service := New(repository, private, &passVerifier{})
			ctx := context.Background()
			name, token := "测试上游", "test-token"
			input := Input{
				Host: "old.example.test", Name: &name, BaseURL: "https://old.example.test",
				AccountBaseURL: "https://models.example.test/v1", UpstreamType: "sub2api",
				AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &token,
				Present: map[string]bool{"access_token": true, "refresh_token": true},
			}
			original, err := service.Create(ctx, input, "tester")
			if err != nil {
				t.Fatal(err)
			}
			input.Host, input.BaseURL = "existing.example.test", "https://existing.example.test"
			if _, err := service.Create(ctx, input, "tester"); err != nil {
				t.Fatal(err)
			}

			input.Host, input.BaseURL = "", address
			_, err = service.Update(ctx, original.Host, input, "tester")
			var invalid *InputError
			if !errors.As(err, &invalid) {
				t.Fatalf("expected input error for invalid or conflicting address, got %v", err)
			}
			unchanged, err := service.Get(ctx, original.Host)
			if err != nil || unchanged.BaseURL != original.BaseURL || unchanged.UpstreamID != original.UpstreamID {
				t.Fatalf("rejected address changed configuration: %#v err=%v", unchanged, err)
			}
			stored, err := private.AuthRecord(ctx, original.Host)
			if err != nil || stored == nil || stored.BaseURL != original.BaseURL {
				t.Fatalf("rejected address changed authentication: %#v err=%v", stored, err)
			}
		})
	}
}
