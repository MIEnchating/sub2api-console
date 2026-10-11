package upstreamconfig_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
)

func TestUpdateToKnownAliasWithoutSubmittedTokensKeepsPreviousAuthentication(t *testing.T) {
	fixture := newAddressFixture(t, "https://"+addressSourceHost)
	fixture.addAlias(t, true)
	publicBefore, err := fixture.repository.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	privateBefore, err := fixture.private.AuthRecord(t.Context(), addressSourceHost)
	if err != nil {
		t.Fatal(err)
	}
	if privateBefore == nil || privateBefore.AccessToken == nil || privateBefore.RefreshToken == nil {
		t.Fatal("the source must have an existing authenticated login")
	}
	fixture.input.Host, fixture.input.BaseURL, fixture.input.AuthMode = "", addressAliasURL, "sub2api_user_token"
	fixture.input.Entry, fixture.input.Present = nil, map[string]bool{}

	_, err = fixture.service.Update(t.Context(), addressSourceHost, fixture.input, "test")
	var invalid *upstreamconfig.InputError
	if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "Token 鉴权必须配置") {
		t.Fatalf("changing the address reused authentication without an explicit login: %v", err)
	}
	publicAfter, err := fixture.repository.Upstreams(t.Context())
	if err != nil || !reflect.DeepEqual(publicBefore, publicAfter) {
		t.Fatalf("missing authentication changed public configuration: %v", err)
	}
	privateAfter, err := fixture.private.AuthRecord(t.Context(), addressSourceHost)
	if err != nil || !reflect.DeepEqual(privateBefore, privateAfter) {
		t.Fatalf("missing authentication changed the previous login: %v", err)
	}
}
