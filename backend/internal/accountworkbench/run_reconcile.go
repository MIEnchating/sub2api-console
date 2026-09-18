package accountworkbench

import (
	"context"
	"errors"
	"maps"

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
	requested := maps.Clone(value.Exports[index])
	if requested == nil {
		return errors.New("原提交配置已不可用，未重复写入")
	}
	if phase == "promoting" {
		requested["status"] = "active"
		requested["schedulable"] = true
	} else {
		requested["status"] = "inactive"
		requested["schedulable"] = false
		if phase == "creating" {
			requested["group_ids"] = []int{}
		}
	}
	if err = verifyApplied(current, requested); err != nil {
		return err
	}
	id := text(current["id"])
	if id == "" {
		return errors.New("站点账号缺少稳定 ID")
	}
	row.AccountID = id
	value.AccountVersions[id] = accountVersion(current)
	value.Phases[row.ID] = "isolated"
	if phase == "creating" {
		row.ImportAction = "created"
	}
	if phase == "updating" {
		row.ImportAction = "updated"
	}
	if phase == "promoting" {
		value.Phases[row.ID] = "completed"
		row.Status = "completed"
		row.Message = "已核对上次启用结果，账号配置与调度均已生效"
	}
	return s.persistRun(value)
}
