package accountworkbench

import (
	"context"
	"errors"
	"net/http"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type AccountActionPreview struct {
	AccountID       string             `json:"account_id"`
	AccountVersion  string             `json:"account_version"`
	TargetVersion   string             `json:"target_version"`
	PlatformPresent bool               `json:"platform_present"`
	Schedulable     bool               `json:"schedulable"`
	Remembered      *RememberedAccount `json:"remembered"`
}

type AccountActionInput struct {
	Action             string `json:"action"`
	AccountVersion     string `json:"account_version"`
	TargetVersion      string `json:"target_version"`
	Schedulable        *bool  `json:"schedulable,omitempty"`
	DeleteRemembered   bool   `json:"delete_remembered"`
	RememberedRevision int64  `json:"remembered_revision"`
}

func (s *Service) AccountActionPreview(ctx context.Context, owner, id string) (AccountActionPreview, error) {
	if err := s.checkOwner(ctx, owner); err != nil {
		return AccountActionPreview{}, err
	}
	preview, _, err := s.accountActionSnapshot(ctx, id)
	return preview, err
}

func (s *Service) accountActionSnapshot(ctx context.Context, id string) (AccountActionPreview, map[string]any, error) {
	result := AccountActionPreview{AccountID: id}
	if !validGroupID(id) {
		return result, nil, errors.New("账号必须使用有效的稳定 ID")
	}
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return result, nil, err
	}
	ctx, err = targetguard.Pin(ctx, s.private)
	if err != nil {
		return result, nil, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return result, nil, err
	}
	client, err := s.client(target)
	if err != nil {
		return result, nil, err
	}
	row, err := client.Account(ctx, id)
	var remoteError *adminclient.HTTPError
	if errors.As(err, &remoteError) && remoteError.StatusCode == http.StatusNotFound {
		result.AccountVersion = "absent"
	} else if err != nil {
		return result, nil, errors.New("平台账号读取失败，请检查站点连接后重试")
	} else {
		if text(row["id"]) != id || text(row["platform"]) != "openai" || text(row["type"]) != "oauth" {
			return result, nil, errors.New("账号 ID 或类型已变化，请刷新列表后重试")
		}
		schedulable, ok := row["schedulable"].(bool)
		if !ok {
			return result, nil, errors.New("平台账号缺少调度状态，请在管理平台核对")
		}
		result.PlatformPresent, result.Schedulable, result.AccountVersion = true, schedulable, accountVersion(row)
	}
	result.TargetVersion = targetguard.Fingerprint(target)
	key := rememberedAccountKey(target, rememberedAccountID(target, id))
	revision, err := s.private.WorkbenchDocumentRevision(ctx, key)
	if err != nil {
		return result, nil, err
	}
	if revision > 0 {
		saved, err := s.readRememberedAccount(ctx, target, rememberedAccountID(target, id))
		if err != nil {
			return result, nil, err
		}
		result.Remembered = &saved.Public
	}
	if _, err = targetguard.Pin(ctx, s.private); err != nil {
		return result, nil, err
	}
	return result, row, nil
}

func validateAccountAction(preview AccountActionPreview, input AccountActionInput) error {
	if input.TargetVersion == "" || input.TargetVersion != preview.TargetVersion {
		return targetguard.ErrChanged
	}
	if input.AccountVersion == "" || input.AccountVersion != preview.AccountVersion {
		return errors.New("平台账号配置已变化，请重新打开确认窗口")
	}
	switch input.Action {
	case "scheduling":
		if input.Schedulable == nil || !preview.PlatformPresent || input.DeleteRemembered || input.RememberedRevision != 0 {
			return errors.New("调度操作参数无效，请刷新后重试")
		}
	case "delete":
		if input.Schedulable != nil {
			return errors.New("删除操作不能同时修改调度")
		}
		if input.DeleteRemembered {
			if preview.Remembered == nil || input.RememberedRevision < 1 || input.RememberedRevision != preview.Remembered.Revision {
				return configstore.ErrWorkbenchVersion
			}
		} else if input.RememberedRevision != 0 {
			return errors.New("未选择删除已记住账号，请重新确认")
		}
	default:
		return errors.New("不支持的账号操作")
	}
	return nil
}

func (s *Service) accountActionAllowed(ctx context.Context, id string, input AccountActionInput) error {
	if err := s.requireAccountManaged(ctx, id); err != nil {
		return err
	}
	if input.Action == "scheduling" && input.Schedulable != nil && *input.Schedulable && s.groupProtection != nil {
		protection, err := s.groupProtection.AccountMutationProtection(ctx, id)
		if err != nil {
			return err
		}
		if protection.Protected() {
			return errors.New("账号受人工控制或排除保护，请先在账号管理解除对应限制")
		}
	}
	return nil
}
