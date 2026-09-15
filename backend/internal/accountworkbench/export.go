package accountworkbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

var (
	ErrExportPreview     = errors.New("导出预览已失效或不属于当前会话，请重新选择账号")
	ErrExportChanged     = errors.New("账号在导出预览后发生变化，请重新预览并确认")
	ErrExportUnavailable = errors.New("私有导出目录不可用，请检查后端配置后重试")
)

type ExportPreviewInput struct {
	AccountIDs []string `json:"account_ids"`
}

type ExportPreviewItem struct {
	AccountID string `json:"account_id"`
	Name      string `json:"name"`
	Revision  string `json:"revision"`
}

type ExportPreview struct {
	ID        string              `json:"id"`
	Revision  string              `json:"revision"`
	ExpiresAt string              `json:"expires_at"`
	Target    string              `json:"target"`
	Items     []ExportPreviewItem `json:"items"`
}

type ExportKind string

const (
	ExportAccounts      ExportKind = "accounts"
	ExportLoginProfiles ExportKind = "login-profiles"
)

type ExportMetadata struct {
	ID        string     `json:"id"`
	Kind      ExportKind `json:"kind"`
	Count     int        `json:"count"`
	CreatedAt string     `json:"created_at"`
	ExpiresAt string     `json:"expires_at"`
}

type preparedExport struct {
	sourceProfileView *SourceProfileExportPreview
	owner             string
	target            configstore.TargetSettings
	view              ExportPreview
	expires           time.Time
	timer             *time.Timer
	kind              ExportKind
	profileView       *ProfileExportPreview
	regeneration      *preparedRegeneration
}

type sub2APIExport struct {
	Type       string           `json:"type"`
	Version    int              `json:"version"`
	ExportedAt string           `json:"exported_at"`
	Proxies    []map[string]any `json:"proxies"`
	Accounts   []map[string]any `json:"accounts"`
}

// UseExportDirectory configures a dedicated directory before serving requests.
// No API accepts a destination path or returns the credential file contents.
func (s *Service) UseExportDirectory(directory string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exportState != nil {
		return errors.New("私有导出目录已配置")
	}
	state, err := openExportState(directory)
	if err != nil {
		return ErrExportUnavailable
	}
	s.exportState = state
	return nil
}

func (s *Service) CloseExports() error {
	s.mu.Lock()
	state := s.exportState
	s.mu.Unlock()
	if state == nil {
		return nil
	}
	return state.close()
}

func (s *Service) exportStorage() (*exportState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exportState == nil {
		return nil, ErrExportUnavailable
	}
	return s.exportState, nil
}

func (s *Service) PreviewExport(ctx context.Context, owner string, input ExportPreviewInput) (ExportPreview, error) {
	if owner == "" {
		return ExportPreview{}, ErrExportPreview
	}
	ids, err := exportAccountIDs(input.AccountIDs)
	if err != nil {
		return ExportPreview{}, err
	}
	state, err := s.exportStorage()
	if err != nil {
		return ExportPreview{}, err
	}
	ctx, err = targetguard.Capture(ctx, s.private)
	if err != nil {
		return ExportPreview{}, err
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, exportResources(ids)...)
	if err != nil {
		return ExportPreview{}, publicError(err)
	}
	defer func() { _ = release() }()
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		return ExportPreview{}, err
	}
	client, target, err := s.client(guarded)
	if err != nil {
		return ExportPreview{}, publicError(err)
	}
	view := ExportPreview{Target: target.BaseURL, Items: make([]ExportPreviewItem, 0, len(ids))}
	for _, id := range ids {
		account, readErr := client.Account(guarded, id)
		if readErr != nil {
			return ExportPreview{}, publicError(readErr)
		}
		if stringValue(account["id"]) != id {
			return ExportPreview{}, errors.New("管理接口返回的账号 ID 与选择不符，请重新同步账号")
		}
		payload, payloadErr := exportAccountPayload(account)
		if payloadErr != nil {
			return ExportPreview{}, payloadErr
		}
		revision, revisionErr := exportRevision(map[string]any{"account_id": id, "account": payload})
		if revisionErr != nil {
			return ExportPreview{}, publicError(revisionErr)
		}
		view.Items = append(view.Items, ExportPreviewItem{AccountID: id, Name: exportAccountName(account, target.AdminKey), Revision: revision})
	}
	if _, err := targetguard.Pin(guarded, s.private); err != nil {
		return ExportPreview{}, err
	}
	view.ID, err = randomID()
	if err != nil {
		return ExportPreview{}, publicError(err)
	}
	view.Revision, err = exportRevision(view.Items)
	if err != nil {
		return ExportPreview{}, publicError(err)
	}
	expires := time.Now().Add(10 * time.Minute)
	view.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	if err := state.addPreview(&preparedExport{owner: exportHash(owner), target: target, view: view, expires: expires, kind: ExportAccounts}); err != nil {
		return ExportPreview{}, err
	}
	return view, nil
}

func (s *Service) DeleteExportPreview(owner, id string) {
	state, err := s.exportStorage()
	if err == nil {
		state.deletePreview(exportHash(owner), id)
	}
}

func (s *Service) Export(ctx context.Context, owner, previewID string, confirmed bool) (taskstore.Task, error) {
	if !confirmed {
		return taskstore.Task{}, errors.New("请先确认导出影响范围")
	}
	if owner == "" {
		return taskstore.Task{}, ErrExportPreview
	}
	state, err := s.exportStorage()
	if err != nil {
		return taskstore.Task{}, err
	}
	prepared, err := state.takePreview(exportHash(owner), previewID, ExportAccounts)
	if err != nil {
		return taskstore.Task{}, err
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, prepared.target), s.private); err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, "account-workbench-export", "等待生成私有导出文件", func(run context.Context, update func([]ResultItem) error) ([]ResultItem, error) {
		row := ResultItem{Index: 0, Name: "私有账号文件", Status: "running", Message: "正在重新校验导出账号"}
		rows := []ResultItem{row}
		if err := update(rows); err != nil {
			return rows, err
		}
		metadata, err := s.writeExport(targetguard.Expect(run, prepared.target), state, prepared)
		if err != nil {
			rows[0].Status = "failed"
			rows[0].Message = "私有导出未生成，请重新预览并核对账号与管理目标"
			return rows, err
		}
		rows[0].Status, rows[0].Message = "succeeded", "私有导出文件已生成"
		rows[0].Report = exportMetadataReport(metadata)
		return rows, nil
	})
}

func exportMetadataReport(metadata ExportMetadata) map[string]any {
	return map[string]any{"artifact_id": metadata.ID, "kind": metadata.Kind, "scope": ScopeManaged, "count": metadata.Count, "created_at": metadata.CreatedAt, "expires_at": metadata.ExpiresAt}
}

func localExportMetadataReport(metadata ExportMetadata) map[string]any {
	report := exportMetadataReport(metadata)
	report["scope"] = ScopeLocalExport
	return report
}

func (s *Service) writeExport(ctx context.Context, state *exportState, prepared *preparedExport) (ExportMetadata, error) {
	ids := make([]string, len(prepared.view.Items))
	for i, item := range prepared.view.Items {
		ids[i] = item.AccountID
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, exportResources(ids)...)
	if err != nil {
		return ExportMetadata{}, err
	}
	defer func() { _ = release() }()
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		return ExportMetadata{}, err
	}
	client, err := s.clientFor(prepared.target)
	if err != nil {
		return ExportMetadata{}, err
	}
	accounts := make([]map[string]any, 0, len(ids))
	totalBytes := 0
	for _, item := range prepared.view.Items {
		account, readErr := client.Account(guarded, item.AccountID)
		if readErr != nil {
			return ExportMetadata{}, readErr
		}
		payload, payloadErr := exportAccountPayload(account)
		if payloadErr != nil {
			return ExportMetadata{}, payloadErr
		}
		revision, revisionErr := exportRevision(map[string]any{"account_id": item.AccountID, "account": payload})
		if revisionErr != nil || stringValue(account["id"]) != item.AccountID || revision != item.Revision {
			return ExportMetadata{}, ErrExportChanged
		}
		encoded, encodeErr := json.Marshal(payload)
		if encodeErr != nil {
			return ExportMetadata{}, encodeErr
		}
		totalBytes += len(encoded)
		if totalBytes > maxExportBytes-4096 {
			return ExportMetadata{}, errors.New("导出文件超过 64 MiB，请分批导出")
		}
		accounts = append(accounts, payload)
	}
	if _, err := targetguard.Pin(guarded, s.private); err != nil {
		return ExportMetadata{}, err
	}
	if err := guarded.Err(); err != nil {
		return ExportMetadata{}, err
	}
	return state.write(guarded, prepared.owner, exportTargetHash(prepared.target), accounts)
}

func (s *Service) Exports(ctx context.Context, owner string) ([]ExportMetadata, error) {
	if owner == "" {
		return nil, ErrExportPreview
	}
	state, err := s.exportStorage()
	if err != nil {
		return nil, err
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return nil, err
	}
	return state.list(exportHash(owner), exportTargetHash(target))
}

func (s *Service) DeleteExport(ctx context.Context, owner, id string) error {
	if owner == "" || !validExportID(id) {
		return ErrExportPreview
	}
	state, err := s.exportStorage()
	if err != nil {
		return err
	}
	ctx, err = targetguard.Capture(ctx, s.private)
	if err != nil {
		return err
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		return err
	}
	target, err := targetguard.Settings(guarded, s.private)
	if err != nil {
		return err
	}
	return state.remove(exportHash(owner), exportTargetHash(target), id)
}

func exportAccountIDs(values []string) ([]string, error) {
	if len(values) < 1 || len(values) > 500 {
		return nil, errors.New("单次导出需要选择 1～500 个账号")
	}
	ids := slices.Clone(values)
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		number, err := strconv.ParseInt(id, 10, 64)
		if err != nil || number < 1 || strconv.FormatInt(number, 10) != id || seen[id] {
			return nil, errors.New("账号 ID 必须是唯一的正整数，请重新选择")
		}
		seen[id] = true
	}
	return ids, nil
}

func exportResources(ids []string) []string {
	resources := make([]string, 0, len(ids)+1)
	resources = append(resources, mutationguard.AccountCatalog())
	for _, id := range ids {
		resources = append(resources, mutationguard.Account(id))
	}
	return resources
}

func exportRevision(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func exportAccountPayload(account map[string]any) (map[string]any, error) {
	config, err := ExtractTemplate(account)
	if err != nil {
		return nil, err
	}
	credentials, ok := account["credentials"].(map[string]any)
	if !ok || (stringValue(credentials["access_token"]) == "" && stringValue(credentials["refresh_token"]) == "") {
		return nil, errors.New("选中账号没有可导出的 OAuth 凭据，请重新授权后再试")
	}
	return ApplyTemplate(InputItem{Name: stringValue(account["name"]), Credentials: credentials}, &configstore.WorkbenchTemplate{Config: config})
}

func exportHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func exportTargetHash(target configstore.TargetSettings) string {
	return exportHash(strings.TrimRight(strings.TrimSpace(target.BaseURL), "/") + "\x00" + strings.TrimSpace(target.AdminKey))
}

func exportAccountName(account map[string]any, adminKey string) string {
	name, _ := account["name"].(string)
	name = redact.Secrets(name)
	if adminKey != "" {
		name = strings.ReplaceAll(name, adminKey, "[已隐藏]")
	}
	var scrub func(any)
	scrub = func(value any) {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				name = strings.ReplaceAll(name, typed, "[已隐藏]")
			}
		case map[string]any:
			for key, child := range typed {
				switch key {
				case "email", "plan_type", "chatgpt_account_id", "account_id", "chatgpt_user_id", "user_id", "organization_id", "expires_at", "expires_in", "token_type", "client_id", "auth_mode":
					continue
				}
				scrub(child)
			}
		case []any:
			for _, child := range typed {
				scrub(child)
			}
		}
	}
	scrub(account["credentials"])
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name))
	if name == "" {
		return fmt.Sprintf("账号 %s", stringValue(account["id"]))
	}
	if runes := []rune(name); len(runes) > 200 {
		return string(runes[:200])
	}
	return name
}
