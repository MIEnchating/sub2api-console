package upstreamconfig_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
)

func TestUpdateToIndependentUpstreamRejectsConflictBeforeRemoteLogin(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		catalog bool
	}{
		{name: "independent-upstream-with-same-display-name", catalog: true},
		{name: "independent-private-authentication-only"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newAddressFixture(t, "https://"+addressSourceHost)
			if scenario.catalog {
				other := fixture.input
				other.Host, other.BaseURL = addressAliasHost, addressAliasURL
				if _, err := fixture.service.Create(t.Context(), other, "test"); err != nil {
					t.Fatal(err)
				}
			} else {
				token, refresh := "other-access-token", "other-refresh-token"
				if err := fixture.private.SaveAuthRecord(t.Context(), configstore.AuthRecord{
					Host: addressAliasHost, BaseURL: addressAliasURL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
					AccessToken: &token, RefreshToken: &refresh,
				}, nil); err != nil {
					t.Fatal(err)
				}
			}
			publicBefore, err := fixture.repository.Upstreams(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			privateBefore, err := fixture.private.AuthRecord(t.Context(), addressSourceHost)
			if err != nil {
				t.Fatal(err)
			}
			otherBefore, err := fixture.private.AuthRecord(t.Context(), addressAliasHost)
			if err != nil {
				t.Fatal(err)
			}
			fixture.verifier.failure = errors.New("remote login must not run for a conflicting Host")

			fixture.input.Host, fixture.input.BaseURL = "", addressAliasURL
			_, err = fixture.service.Update(t.Context(), addressSourceHost, fixture.input, "test")
			if !errors.Is(err, upstreamconfig.ErrConflict) || !strings.Contains(err.Error(), addressAliasHost) || !strings.Contains(err.Error(), "请") {
				t.Fatalf("conflicting independent identity was not rejected with an actionable conflict: %v", err)
			}
			publicAfter, err := fixture.repository.Upstreams(t.Context())
			if err != nil || !reflect.DeepEqual(publicBefore, publicAfter) {
				t.Fatalf("conflicting address changed public configuration: %v", err)
			}
			privateAfter, err := fixture.private.AuthRecord(t.Context(), addressSourceHost)
			if err != nil || !reflect.DeepEqual(privateBefore, privateAfter) {
				t.Fatalf("conflicting address changed private configuration: %v", err)
			}
			otherAfter, err := fixture.private.AuthRecord(t.Context(), addressAliasHost)
			if err != nil || !reflect.DeepEqual(otherBefore, otherAfter) {
				t.Fatalf("conflicting address changed the target authentication: %v", err)
			}
		})
	}
}
