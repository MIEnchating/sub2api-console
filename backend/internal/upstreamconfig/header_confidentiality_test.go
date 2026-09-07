package upstreamconfig

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestConfigurationResponseExposesHeaderNamesWithoutCredentialValues(t *testing.T) {
	private, repository := openStores(t)
	name := "Custom upstream"
	if _, err := repository.CreateUpstreamConfiguration(context.Background(), business.UpstreamConfigurationWrite{Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "custom", AuthMode: "custom_headers", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := private.SaveAuthRecord(context.Background(), configstore.AuthRecord{Host: "api.example", BaseURL: "https://api.example", UpstreamType: "custom", AuthMode: "custom_headers", Headers: map[string]string{"Authorization": "Bearer private-token", "X-Custom-Secret": "private-api-key"}}, allAuthFields()); err != nil {
		t.Fatal(err)
	}
	service := New(repository, private, &passVerifier{})
	configuration, err := service.Get(context.Background(), "api.example")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private-token") || strings.Contains(string(encoded), "private-api-key") || len(configuration.Headers) != 0 {
		t.Fatal("configuration response exposed saved custom header credential values")
	}
	if len(configuration.HeaderNames) != 2 || configuration.HeaderNames[0] != "Authorization" || configuration.HeaderNames[1] != "X-Custom-Secret" {
		t.Fatalf("header names=%v", configuration.HeaderNames)
	}
}

func TestConfigurationUpdatePreservesHeadersWhenOmittedAndClearsExplicitEmptyMap(t *testing.T) {
	private, repository := openStores(t)
	name, token, refresh := "Example", "access", "refresh"
	service := New(repository, private, &passVerifier{})
	input := Input{Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &refresh, Headers: map[string]string{"Authorization": "Bearer private-token"}, Present: map[string]bool{"access_token": true, "refresh_token": true, "headers": true}}
	if _, err := service.Create(context.Background(), input, "operator"); err != nil {
		t.Fatal(err)
	}
	input.Headers, input.Present = map[string]string{}, map[string]bool{}
	if _, err := service.Update(context.Background(), "api.example", input, "operator"); err != nil {
		t.Fatal(err)
	}
	stored, err := private.AuthRecord(context.Background(), "api.example")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Headers["Authorization"] != "Bearer private-token" {
		t.Fatal("omitting headers lost saved credentials")
	}
	input.Present["headers"] = true
	if _, err := service.Update(context.Background(), "api.example", input, "operator"); err != nil {
		t.Fatal(err)
	}
	stored, err = private.AuthRecord(context.Background(), "api.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Headers) != 0 {
		t.Fatal("explicit empty headers did not clear credentials")
	}
}
