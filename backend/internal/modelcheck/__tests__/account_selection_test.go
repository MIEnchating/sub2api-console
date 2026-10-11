package modelcheck_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

type scopedSelectionCatalog struct {
	*catalog
	lists     int
	detailErr error
}

func (c *scopedSelectionCatalog) Account(ctx context.Context, id string) (*business.AccountDetail, error) {
	if c.detailErr != nil {
		return nil, c.detailErr
	}
	return c.catalog.Account(ctx, id)
}

func (c *scopedSelectionCatalog) Accounts(context.Context) ([]business.AccountStatus, error) {
	c.lists++
	return nil, errors.New("full catalog must not be read for explicit account IDs")
}

func TestSelectedAccountReadFailurePreservesMissingAccountReason(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "deleted in storage", err: sql.ErrNoRows, want: "不存在"},
		{name: "storage unavailable", err: errors.New("isolated storage failure"), want: "详情读取失败"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unreadable account reached upstream") })
			c := &scopedSelectionCatalog{catalog: f.catalog, detailErr: tc.err}
			service, err := modelcheck.New(f.tasks, credentials{}, c, credentials{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.EnqueueAnimation(t.Context(), request("1"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("account read error=%v; want %s", err, tc.want)
			}
		})
	}
}

func TestSavingSelectedSchedulesReadsOnlySelectedAccountDetails(t *testing.T) {
	t.Parallel()
	f := setup(t, 2, "openai", func(http.ResponseWriter, *http.Request) { t.Error("schedule validation made a network request") })
	c := &scopedSelectionCatalog{catalog: f.catalog}
	service, err := modelcheck.New(f.tasks, credentials{}, c, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	values, err := service.SaveAnimationSchedules(t.Context(), []modelcheck.AnimationSchedule{
		{AccountID: "1", Model: "gpt-6-astra", Enabled: true, IntervalMinutes: 5, TimeoutSeconds: 5},
		{AccountID: "2", Model: "gpt-6-astra", Enabled: true, IntervalMinutes: 5, TimeoutSeconds: 5},
	}, "test")
	if err != nil || len(values) != 2 || c.lists != 0 {
		t.Fatalf("selected schedule validation read the full catalog: values=%+v lists=%d err=%v", values, c.lists, err)
	}
}

func TestSelectedAccountRejectsMissingOrMismatchedDetailBeforeCreatingTask(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		want string
	}{
		{name: "deleted account", want: "不存在"},
		{name: "wrong stable identity", id: "2", want: "不匹配"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("invalid account reached upstream") })
			if tc.id == "" {
				delete(f.catalog.details, "1")
			} else {
				f.catalog.details["1"].ID = tc.id
			}
			_, err := f.service.EnqueueAnimation(t.Context(), request("1"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("invalid selected detail error=%v; want %s", err, tc.want)
			}
		})
	}
}
