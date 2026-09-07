package upstreamconfig

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type cancellingHostRenamePrivate struct {
	*configstore.Store
	cancel  context.CancelFunc
	failure error
}

func (private *cancellingHostRenamePrivate) RenameAuthRecord(context.Context, string, string) error {
	private.cancel()
	return private.failure
}

type failingHostRollbackBusiness struct{ *business.Store }

func (repository *failingHostRollbackBusiness) RenameUpstreamHost(ctx context.Context, oldHost, newHost string) error {
	if oldHost == "new.example" {
		return errors.New("host rollback unavailable")
	}
	return repository.Store.RenameUpstreamHost(ctx, oldHost, newHost)
}

func TestHostRenameCompensatesPublicIdentityAfterRequestCancellation(t *testing.T) {
	for _, rollbackFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "restore", true: "report-rollback-error"}[rollbackFails], func(t *testing.T) {
			private, repository := openStores(t)
			name, token := "Example", "test-token"
			input := Input{Host: "old.example", Name: &name, BaseURL: "https://old.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1", AccessToken: &token, RefreshToken: &token, Present: map[string]bool{"access_token": true, "refresh_token": true}}
			if _, err := New(repository, private, &passVerifier{}).Create(context.Background(), input, "operator"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("private rename failed")
			var public Business = repository
			if rollbackFails {
				public = &failingHostRollbackBusiness{Store: repository}
			}
			service := New(public, &cancellingHostRenamePrivate{Store: private, cancel: cancel, failure: failure}, &passVerifier{})
			input.Host, input.BaseURL = "new.example", "https://new.example"
			_, err := service.Update(ctx, "old.example", input, "operator")
			if !errors.Is(err, failure) {
				t.Fatalf("rename error=%v", err)
			}
			if rollbackFails {
				if !strings.Contains(err.Error(), "host rollback unavailable") {
					t.Fatalf("rollback failure was hidden: %v", err)
				}
				return
			}
			oldExists, readErr := repository.UpstreamExists(context.Background(), "old.example")
			if readErr != nil || !oldExists {
				t.Fatalf("public identity was not restored: exists=%v err=%v", oldExists, readErr)
			}
			newExists, readErr := repository.UpstreamExists(context.Background(), "new.example")
			if readErr != nil || newExists {
				t.Fatalf("failed destination remains: exists=%v err=%v", newExists, readErr)
			}
		})
	}
}
