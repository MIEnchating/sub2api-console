package upstreamsync_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestBalanceRecoveryRunsOnlyAfterFreshBalanceAndReportsRecoveryFailure(t *testing.T) {
	for _, scenario := range []string{"fresh", "missing", "read-failed", "catalog-only", "recovery-failed"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newBalanceFixture(t, balanceReader{read: func(context.Context, string) (business.UpstreamBalanceObservation, error) {
				if scenario == "read-failed" {
					return business.UpstreamBalanceObservation{}, errors.New("balance unavailable")
				}
				if scenario == "missing" {
					return business.UpstreamBalanceObservation{Status: "未返回余额"}, nil
				}
				balance := "20"
				return business.UpstreamBalanceObservation{RawBalance: &balance, Status: "已读取"}, nil
			}}, "recharged.example")
			calls := 0
			fixture.syncer.UseBalanceRecovery(func(ctx context.Context, host, actor string) (bool, error) {
				calls++
				if host != "recharged.example" || actor != "balance-test" {
					t.Fatalf("wrong recovery scope: %s %s", host, actor)
				}
				// Reacquisition proves the synchronization lease was released first.
				_, release, err := mutationguard.Acquire(ctx, fixture.store, mutationguard.Upstream(host))
				if err != nil {
					return false, err
				}
				defer release()
				var balance string
				if err := fixture.db.QueryRow(`SELECT raw_balance FROM upstreams WHERE host=?`, host).Scan(&balance); err != nil {
					return false, err
				}
				if balance != "20" {
					t.Fatalf("recovery preceded balance commit: %s", balance)
				}
				if scenario == "recovery-failed" {
					return true, errors.New("clear-error rejected")
				}
				return true, nil
			})
			scope := upstreamsync.Scope{Balance: true}
			if scenario == "catalog-only" {
				scope = upstreamsync.Scope{Catalog: true}
			}
			result, err := fixture.syncer.SyncHost(t.Context(), "recharged.example", scope, "balance-test")
			if err != nil {
				t.Fatal(err)
			}
			expected := 0
			if scenario == "fresh" || scenario == "recovery-failed" {
				expected = 1
			}
			if result.RemoteWrite != (expected == 1) {
				t.Fatal("task remote-write status does not reflect recovery")
			}
			if calls != expected {
				t.Fatalf("recovery calls=%d want=%d", calls, expected)
			}
			if scenario == "recovery-failed" && (result.Status != "failed" || result.Reason == nil || !strings.Contains(*result.Reason, "账号状态恢复失败")) {
				t.Fatalf("failure hidden: %+v", result)
			}
		})
	}
}
