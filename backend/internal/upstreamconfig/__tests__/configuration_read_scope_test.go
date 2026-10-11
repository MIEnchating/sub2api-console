package upstreamconfig_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
)

func TestConfigurationReadIgnoresUnrelatedUpstreamSpending(t *testing.T) {
	f := configurationWithUnrelatedInvalidSpending(t)
	result, err := f.service.Get(t.Context(), addressSourceHost)
	if err != nil {
		t.Fatalf("healthy upstream configuration must be readable: %v", err)
	}
	if result.UpstreamID != f.original.UpstreamID || result.AccountBaseURL != f.original.AccountBaseURL {
		t.Fatalf("configuration changed: %+v", result)
	}
	if _, err := f.repository.Upstreams(t.Context()); err == nil {
		t.Fatal("full summary must still report invalid spending")
	}
}

func configurationWithUnrelatedInvalidSpending(t *testing.T) *addressFixture {
	t.Helper()
	f := newAddressFixture(t, "https://"+addressSourceHost)
	name := "其他上游"
	_, err := f.repository.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "other.example.test", Name: &name, BaseURL: "https://other.example.test",
		UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(t.Context(), `UPDATE upstreams SET metadata_json='{"balance_spending":"invalid"}' WHERE host='other.example.test'`); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestAddressUpdateIgnoresUnrelatedUpstreamSpending(t *testing.T) {
	f := configurationWithUnrelatedInvalidSpending(t)
	input := f.input
	input.Host, input.BaseURL = "renamed.example.test", "https://renamed.example.test"
	updated, err := f.service.Update(t.Context(), addressSourceHost, input, "test")
	if err != nil || updated.UpstreamID != f.original.UpstreamID || updated.Host != input.Host {
		t.Fatalf("unrelated spending blocked address update: %+v, %v", updated, err)
	}
}

type rejectingAuthVerifier struct{ called bool }

func (v *rejectingAuthVerifier) Verify(context.Context, configstore.AuthRecord) error {
	v.called = true
	return errors.New("unexpected remote authentication")
}

func (v *rejectingAuthVerifier) Login(_ context.Context, record configstore.AuthRecord, _ configstore.VaultEntry) (configstore.AuthRecord, error) {
	v.called = true
	return record, errors.New("unexpected remote authentication")
}

func TestInvalidAccountBaseURLRejectedBeforeRemoteAuthentication(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			f := newAddressFixture(t, "https://"+addressSourceHost)
			verifier := &rejectingAuthVerifier{}
			service := upstreamconfig.New(f.repository, f.private, verifier)
			input := f.input
			input.AccountBaseURL = "file:///tmp/invalid"
			var err error
			if operation == "create" {
				input.Host, input.BaseURL = "new.example.test", "https://new.example.test"
				_, err = service.Create(t.Context(), input, "test")
			} else {
				_, err = service.Update(t.Context(), addressSourceHost, input, "test")
			}
			var inputErr *upstreamconfig.InputError
			if !errors.As(err, &inputErr) || !strings.Contains(err.Error(), "账号 Base URL 无效") {
				t.Fatalf("expected account URL input error, got %v", err)
			}
			if verifier.called {
				t.Fatal("invalid local input reached remote authentication")
			}
			current, err := f.service.Get(t.Context(), addressSourceHost)
			if err != nil || current.AccountBaseURL != f.original.AccountBaseURL {
				t.Fatalf("invalid input changed saved configuration: %+v, %v", current, err)
			}
		})
	}
}
