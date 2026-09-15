package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

var errRegenerationSource = errors.New("再生来源已变化、到期或没有可用账号，请重新选择来源并预览")

func (s *Service) loadRegenerationSource(ctx context.Context, state *exportState, owner string, target configstore.TargetSettings, input RegenerationInput) (regenerationSource, error) {
	source := regenerationSource{}
	if len(input.AccountIDs) > 0 {
		client, err := s.clientFor(target)
		if err != nil {
			return source, err
		}
		for index, id := range input.AccountIDs {
			account, err := client.Account(ctx, id)
			if err != nil {
				return source, publicError(err)
			}
			if stringValue(account["id"]) != id {
				return source, errRegenerationSource
			}
			snapshot, err := regenerationSnapshot(index, id, account, target)
			if err != nil {
				return source, err
			}
			source.accounts = append(source.accounts, snapshot)
		}
	} else {
		var err error
		source, err = s.regenerationTaskSource(ctx, state, owner, target, input.SourceTaskID, ScopeManaged)
		if err != nil {
			return source, err
		}
	}
	return selectRegenerationSource(source, input)
}

func selectRegenerationSource(source regenerationSource, input RegenerationInput) (regenerationSource, error) {
	if len(input.Indexes) > 0 {
		selected := make([]regenerationAccount, 0, len(input.Indexes))
		for _, index := range input.Indexes {
			found := false
			for _, account := range source.accounts {
				if account.view.Index == index {
					selected = append(selected, account)
					found = true
					break
				}
			}
			if !found {
				return source, errRegenerationSource
			}
		}
		source.accounts = selected
	}
	if len(source.accounts) < 1 || len(source.accounts) > 500 {
		return source, errRegenerationSource
	}
	seen := make(map[string]bool, len(source.accounts))
	for _, account := range source.accounts {
		identity := IdentityKey(InputItem{Credentials: inputObject(account.payload["credentials"])})
		if seen[identity] {
			return source, errors.New("所选来源包含重复的官方用户及工作区，请只保留一项")
		}
		seen[identity] = true
	}
	return source, nil
}

func (s *Service) regenerationTaskSource(ctx context.Context, state *exportState, owner string, target configstore.TargetSettings, id string, scope ExportScope) (regenerationSource, error) {
	reader, ok := s.tasks.(interface {
		Get(context.Context, string) (taskstore.Task, error)
	})
	if !ok {
		return regenerationSource{}, errRegenerationSource
	}
	task, err := reader.Get(ctx, id)
	if err != nil || task.Skill != Skill || (task.Status != "succeeded" && task.Status != "partial" && task.Status != "failed" && task.Status != "cancelled") {
		return regenerationSource{}, errRegenerationSource
	}
	taskRevision, err := exportRevision(task)
	if err != nil {
		return regenerationSource{}, errRegenerationSource
	}
	if task.Operation == "account-workbench-import" || task.Operation == "account-workbench-retry" {
		if scope == ScopeLocalExport {
			return regenerationSource{}, errRegenerationSource
		}
		return s.regenerationImportSource(ctx, target, task.ID, taskRevision)
	}
	if task.Operation != "account-workbench-export" && task.Operation != "account-workbench-convert" && task.Operation != "account-workbench-regenerate" {
		return regenerationSource{}, errRegenerationSource
	}
	var result struct {
		Items []struct {
			Index  int    `json:"index"`
			Status string `json:"status"`
			Report struct {
				ArtifactID string     `json:"artifact_id"`
				Kind       ExportKind `json:"kind"`
			} `json:"report"`
		} `json:"items"`
	}
	raw, err := json.Marshal(task.Result)
	if err != nil || json.Unmarshal(raw, &result) != nil {
		return regenerationSource{}, errRegenerationSource
	}
	source := regenerationSource{revision: taskRevision}
	seen := make(map[string]bool)
	for _, row := range result.Items {
		if row.Status != "succeeded" || row.Report.Kind != ExportAccounts || seen[row.Report.ArtifactID] {
			continue
		}
		seen[row.Report.ArtifactID] = true
		namespace := exportTargetHash(target)
		if scope == ScopeLocalExport {
			namespace = workbenchScopeFingerprint(scope, target)
		}
		accounts, metadata, err := state.readAccountArtifact(owner, namespace, row.Report.ArtifactID)
		if err != nil {
			return source, err
		}
		expires, _ := time.Parse(time.RFC3339Nano, metadata.ExpiresAt)
		if source.expires.IsZero() || expires.Before(source.expires) {
			source.expires = expires
		}
		if task.Operation == "account-workbench-regenerate" && (len(accounts) != 1 || row.Index < 0 || row.Index >= 500) {
			return source, errRegenerationSource
		}
		for _, account := range accounts {
			if len(source.accounts) >= 500 {
				return source, errRegenerationSource
			}
			index := len(source.accounts)
			if task.Operation == "account-workbench-regenerate" || (scope == ScopeLocalExport && task.Operation == "account-workbench-convert" && len(accounts) == 1) {
				index = row.Index
			}
			snapshot, err := regenerationSnapshot(index, "", account, target)
			if err != nil {
				return source, err
			}
			source.accounts = append(source.accounts, snapshot)
		}
	}
	return source, nil
}

// Import history contributes stable account IDs only. Saved raw input credentials
// are not reused: current accounts must match the original official identities.
func (s *Service) regenerationImportSource(ctx context.Context, target configstore.TargetSettings, id, taskRevision string) (regenerationSource, error) {
	store, ok := s.private.(executionStore)
	if !ok {
		return regenerationSource{}, errRegenerationSource
	}
	record, err := store.WorkbenchExecution(ctx, id)
	if err != nil || record.TargetURL != target.BaseURL || record.TargetFingerprint != executionTargetFingerprint(target) {
		return regenerationSource{}, errRegenerationSource
	}
	revision, err := exportRevision([]any{taskRevision, record.Revision})
	if err != nil {
		return regenerationSource{}, errRegenerationSource
	}
	expires, _ := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	source := regenerationSource{revision: revision, expires: expires}
	client, err := s.clientFor(target)
	if err != nil {
		return source, err
	}
	for _, saved := range record.Items {
		if saved.AccountID == "" {
			continue
		}
		account, err := client.Account(ctx, saved.AccountID)
		if err != nil || stringValue(account["id"]) != saved.AccountID {
			return source, errRegenerationSource
		}
		snapshot, err := regenerationSnapshot(saved.Index, saved.AccountID, account, target)
		if err != nil {
			return source, err
		}
		if IdentityKey(InputItem{Credentials: inputObject(snapshot.payload["credentials"])}) != saved.Identity {
			return source, errRegenerationSource
		}
		source.accounts = append(source.accounts, snapshot)
	}
	return source, nil
}

func (s *exportState) readAccountArtifact(owner, target, id string) ([]map[string]any, ExportMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact := s.artifacts[id]
	if s.closed || !validExportID(id) || artifact == nil || artifact.Owner != owner || artifact.Target != target || artifact.Metadata.Kind != ExportAccounts {
		return nil, ExportMetadata{}, errRegenerationSource
	}
	expires, err := time.Parse(time.RFC3339Nano, artifact.Metadata.ExpiresAt)
	if err != nil || !time.Now().Before(expires) {
		return nil, ExportMetadata{}, errRegenerationSource
	}
	file, err := s.openExisting(id + ".json")
	if err != nil {
		return nil, ExportMetadata{}, errRegenerationSource
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxExportBytes+1))
	if err != nil || len(raw) > maxExportBytes {
		return nil, ExportMetadata{}, errRegenerationSource
	}
	var data struct {
		Type     string            `json:"type"`
		Version  int               `json:"version"`
		Accounts []json.RawMessage `json:"accounts"`
	}
	if json.Unmarshal(raw, &data) != nil || data.Type != "sub2api-data" || data.Version != 1 || len(data.Accounts) != artifact.Metadata.Count {
		return nil, ExportMetadata{}, errRegenerationSource
	}
	accounts := make([]map[string]any, 0, len(data.Accounts))
	for _, raw := range data.Accounts {
		value, err := decodeInputJSON(string(raw))
		account, ok := value.(map[string]any)
		if err != nil || !ok {
			return nil, ExportMetadata{}, errRegenerationSource
		}
		accounts = append(accounts, account)
	}
	return accounts, artifact.Metadata, nil
}
