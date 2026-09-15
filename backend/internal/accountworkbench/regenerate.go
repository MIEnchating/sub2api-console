package accountworkbench

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

const exportRegeneration ExportKind = "regenerate"

type RegenerationInput struct {
	Scope        ExportScope `json:"scope,omitempty"`
	ArtifactID   string      `json:"artifact_id,omitempty"`
	AccountIDs   []string    `json:"account_ids,omitempty"`
	SourceTaskID string      `json:"source_task_id,omitempty"`
	Indexes      []int       `json:"indexes,omitempty"`
}

type RegenerationItem struct {
	Index       int    `json:"index"`
	AccountID   string `json:"account_id,omitempty"`
	Name        string `json:"name"`
	Email       string `json:"email,omitempty"`
	UserID      string `json:"user_id"`
	WorkspaceID string `json:"workspace_id"`
	Revision    string `json:"revision"`
}

type RegenerationPreview struct {
	Scope        ExportScope        `json:"scope"`
	ArtifactID   string             `json:"artifact_id,omitempty"`
	ID           string             `json:"id"`
	Target       string             `json:"target"`
	SourceTaskID string             `json:"source_task_id,omitempty"`
	ExpiresAt    string             `json:"expires_at"`
	Items        []RegenerationItem `json:"items"`
}

type regenerationAccount struct {
	view    RegenerationItem
	payload map[string]any
}

type regenerationSource struct {
	accounts []regenerationAccount
	revision string
	expires  time.Time
}

type preparedRegeneration struct {
	input          RegenerationInput
	view           RegenerationPreview
	sourceRevision string
}

// PreviewRegeneration is read-only. No refresh token is sent until confirmation.
func (s *Service) PreviewRegeneration(ctx context.Context, owner string, input RegenerationInput) (RegenerationPreview, error) {
	if owner == "" {
		return RegenerationPreview{}, ErrExportPreview
	}
	input, err := validateRegenerationInput(input)
	if err != nil {
		return RegenerationPreview{}, err
	}
	if input.Scope == ScopeLocalExport {
		return s.previewLocalRegeneration(ctx, owner, input)
	}
	state, err := s.exportStorage()
	if err != nil {
		return RegenerationPreview{}, err
	}
	ctx, err = targetguard.Capture(ctx, s.private)
	if err != nil {
		return RegenerationPreview{}, err
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository)
	if err != nil {
		return RegenerationPreview{}, err
	}
	defer func() { _ = release() }()
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		return RegenerationPreview{}, err
	}
	target, err := targetguard.Settings(guarded, s.private)
	if err != nil {
		return RegenerationPreview{}, err
	}
	source, err := s.loadRegenerationSource(guarded, state, exportHash(owner), target, input)
	if err != nil {
		return RegenerationPreview{}, err
	}
	view := RegenerationPreview{Scope: ScopeManaged, Target: target.BaseURL, SourceTaskID: input.SourceTaskID, Items: make([]RegenerationItem, 0, len(source.accounts))}
	for _, account := range source.accounts {
		view.Items = append(view.Items, account.view)
	}
	expires := time.Now().Add(10 * time.Minute)
	if !source.expires.IsZero() && source.expires.Before(expires) {
		expires = source.expires
	}
	view.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	view.ID, err = randomID()
	if err != nil {
		return RegenerationPreview{}, err
	}
	if _, err := targetguard.Pin(guarded, s.private); err != nil {
		return RegenerationPreview{}, err
	}
	stored := view
	stored.Items = slices.Clone(view.Items)
	prepared := &preparedExport{owner: exportHash(owner), target: target, view: ExportPreview{ID: view.ID}, expires: expires, kind: exportRegeneration, regeneration: &preparedRegeneration{input: input, view: stored, sourceRevision: source.revision}}
	if err := state.addPreview(prepared); err != nil {
		return RegenerationPreview{}, err
	}
	return view, nil
}

func validateRegenerationInput(input RegenerationInput) (RegenerationInput, error) {
	var err error
	input.Scope, err = normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return input, err
	}
	if input.Scope == ScopeLocalExport {
		if len(input.AccountIDs) > 0 || (input.SourceTaskID == "") == (input.ArtifactID == "") {
			return input, errors.New("独立再生只能选择一个本地私有文件或已结束的转换任务")
		}
	} else if input.ArtifactID != "" || (len(input.AccountIDs) == 0) == (input.SourceTaskID == "") {
		return input, errors.New("请选择线上账号或一个已结束任务作为再生来源")
	}
	if input.SourceTaskID != "" || input.ArtifactID != "" {
		id := input.SourceTaskID
		if id == "" {
			id = input.ArtifactID
		}
		if !validExportID(id) {
			return input, errors.New("来源任务 ID 无效")
		}
		if len(input.Indexes) > 500 {
			return input, errors.New("单次再生最多选择 500 项")
		}
		seen := make(map[int]bool, len(input.Indexes))
		for _, index := range input.Indexes {
			if index < 0 || index >= 500 || seen[index] {
				return input, errors.New("来源项目序号无效或重复")
			}
			seen[index] = true
		}
		input.Indexes = slices.Clone(input.Indexes)
		return input, nil
	}
	if len(input.Indexes) > 0 {
		return input, errors.New("线上账号来源不能指定文件项目序号")
	}
	ids, err := exportAccountIDs(input.AccountIDs)
	input.AccountIDs = ids
	return input, err
}

func regenerationSnapshot(index int, id string, account map[string]any, target configstore.TargetSettings) (regenerationAccount, error) {
	payload, err := exportAccountPayload(account)
	if err != nil {
		return regenerationAccount{}, err
	}
	credentials := cloneInputMap(inputObject(payload["credentials"]))
	copyInputIdentity(credentials, inputObject(account["extra"]))
	copyInputIdentity(credentials, account)
	enrichInputJWT(credentials)
	identity := inputIdentity(credentials)
	refresh := inputText(credentials["refresh_token"])
	if refresh == "" || len(refresh) > 65536 || strings.Contains(refresh, "***") || identity.user == "" || identity.workspace == "" {
		return regenerationAccount{}, fmt.Errorf("第 %d 项缺少完整的 RT、官方用户 ID 或工作区 ID，请重新授权", index+1)
	}
	secrets := templateSourceSecrets(account, target.AdminKey)
	for _, value := range []string{identity.user, identity.workspace, inputText(credentials["email"])} {
		if len(value) > 2048 || strings.ContainsAny(value, "\r\n") || templateTextHasSecret(value, secrets) {
			return regenerationAccount{}, errors.New("账号身份字段无效，请重新授权后再生成文件")
		}
	}
	payload["credentials"] = credentials
	revision, err := exportRevision(payload)
	if err != nil {
		return regenerationAccount{}, errors.New("无法读取账号版本")
	}
	return regenerationAccount{payload: payload, view: RegenerationItem{Index: index, AccountID: id, Name: exportAccountName(account, target.AdminKey), Email: inputText(credentials["email"]), UserID: identity.user, WorkspaceID: identity.workspace, Revision: revision}}, nil
}
