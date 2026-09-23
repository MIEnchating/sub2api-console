package accountworkbench

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

// reconcileItem only reads the site. An uncertain write is considered complete
// only when stable identity, credentials and all requested fields match.
func (s *Service) reconcileItem(ctx context.Context, value *privateRun, index int) error {
	row := &value.Public.Items[index]
	phase := value.Phases[row.ID]
	resources := []string{mutationguard.AccountCatalog()}
	if row.AccountID != "" {
		resources = append(resources, mutationguard.Account(row.AccountID))
	}
	ctx, release, err := targetguard.Acquire(targetguard.Expect(ctx, value.Target), s.private, resources...)
	if err != nil {
		return errors.New("账号正在变更，暂时无法核对")
	}
	defer release()
	if err = s.executionAllowed(ctx, value); err != nil {
		return err
	}
	client, err := s.client(value.Target)
	if err != nil {
		return errors.New("站点配置无法读取")
	}
	item := value.Items[index].Item
	item.Credentials = value.Items[index].Credentials
	var current map[string]any
	if row.AccountID != "" {
		current, err = client.Account(ctx, row.AccountID)
		var httpErr *adminclient.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return fmt.Errorf("原账号 #%s 已不存在；如需导入，请重新导入并核对当前账号，不能继续旧记录", row.AccountID)
		}
	} else {
		accounts, readErr := client.Accounts(ctx)
		if readErr != nil {
			return errors.New("站点账号读取失败，未重复写入")
		}
		current, err = FindExistingAccount(item, accounts)
	}
	if err != nil || current == nil || !sameAccountIdentity(current, item.Credentials) {
		return errors.New("原提交尚未核对，请在站点确认账号，未重复写入")
	}
	if phase == "isolating" {
		if current["schedulable"] != false || accountVersion(current) != value.AccountVersions[row.AccountID] {
			return errors.New("停止调度结果或账号配置未核对，未继续写入配置")
		}
		// Only the pause was submitted. Import can now re-read the directory
		// and perform the still-unsubmitted configuration update.
		row.AccountID = ""
		value.Phases[row.ID] = "materialized"
		return s.persistRun(value)
	}
	requested := maps.Clone(value.Exports[index])
	if requested == nil {
		return errors.New("原提交配置已不可用，未重复写入")
	}
	completed := false
	if phase == "promoting" || (value.Settings.Promote && (!value.Settings.Check || passedImportCheck(row.Check))) {
		// Always compare the final template groups, including after creation.
		requested["status"] = "active"
		requested["schedulable"] = true
		completed = verifyApplied(current, requested) == nil
	}
	if !completed {
		if phase == "promoting" {
			return verifyApplied(current, requested)
		}
		requested["status"] = "active"
		requested["schedulable"] = false
		if phase == "creating" {
			requested["group_ids"] = []int{}
		}
		if phase == "configuring" {
			err = verifyApplied(current, requested)
		} else {
			err = verifyAppliedWhileIsolated(current, requested)
		}
		if err != nil {
			return err
		}
	}
	id := text(current["id"])
	if id == "" {
		return errors.New("站点账号缺少稳定 ID")
	}
	row.AccountID = id
	value.AccountVersions[id] = accountVersion(current)
	value.Phases[row.ID] = "isolated"
	if phase == "configuring" {
		value.Phases[row.ID] = "configured"
	}
	if phase == "creating" {
		row.ImportAction = "created"
	}
	if phase == "updating" {
		row.ImportAction = "updated"
	}
	if completed {
		value.Phases[row.ID] = "completed"
		row.Status = "completed"
		row.Message = "已核对上次启用结果，账号配置与调度均已生效"
	}
	return s.persistRun(value)
}
