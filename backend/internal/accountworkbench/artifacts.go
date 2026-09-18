package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Artifact struct {
	ID        string    `json:"id"`
	Count     int       `json:"count"`
	ExpiresAt time.Time `json:"expires_at"`
}
type artifactRecord struct {
	Artifact
	Owner       string `json:"owner"`
	RunID       string `json:"run_id"`
	File        string `json:"file"`
	RunRevision int64  `json:"run_revision"`
}

func (s *Service) Export(ctx context.Context, owner string, input RunConfirmation) (Artifact, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	value, err := s.confirmRun(ctx, owner, input)
	if err != nil {
		return Artifact{}, err
	}
	if !value.Public.ExpiresAt.After(time.Now().UTC()) {
		return Artifact{}, ErrRun
	}
	s.activeMu.Lock()
	active := s.active[input.ID] != nil
	s.activeMu.Unlock()
	if active {
		return Artifact{}, errors.New("请等待本批处理结束后导出")
	}
	accounts := []map[string]any{}
	for _, account := range value.Exports {
		if account != nil {
			accounts = append(accounts, account)
		}
	}
	if len(accounts) == 0 {
		return Artifact{}, errors.New("本批尚无可导出的账号")
	}
	if s.artifactDirectory == "" {
		return Artifact{}, errors.New("私有文件目录尚未配置")
	}
	previous, err := s.private.WorkbenchDocuments(ctx, "artifact:"+owner+":"+input.ID+":", 501)
	if err != nil {
		return Artifact{}, err
	}
	for _, record := range previous {
		var saved artifactRecord
		if json.Unmarshal(record.Payload, &saved) != nil || saved.Owner != owner || saved.RunID != input.ID {
			return Artifact{}, errors.New("私有文件索引无效")
		}
		if saved.RunRevision == input.Revision && saved.ExpiresAt.After(time.Now().UTC()) && saved.File == saved.ID+".json" && filepath.Base(saved.File) == saved.File {
			info, statErr := os.Lstat(filepath.Join(s.artifactDirectory, saved.File))
			if statErr == nil && info.Mode().IsRegular() && info.Mode().Perm() == 0600 {
				return saved.Artifact, nil
			}
		}
	}
	if err = s.deleteArtifacts(ctx, owner, input.ID); err != nil {
		return Artifact{}, err
	}
	if err = os.MkdirAll(s.artifactDirectory, 0700); err != nil {
		return Artifact{}, errors.New("私有文件目录不可写")
	}
	info, err := os.Lstat(s.artifactDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Artifact{}, errors.New("私有文件目录无效")
	}
	if err = os.Chmod(s.artifactDirectory, 0700); err != nil {
		return Artifact{}, errors.New("私有文件目录权限设置失败")
	}
	artifact := Artifact{ID: newID(), Count: len(accounts), ExpiresAt: value.Public.ExpiresAt}
	filename := artifact.ID + ".json"
	raw, err := json.MarshalIndent(map[string]any{"type": "sub2api-data", "version": 1, "exported_at": time.Now().UTC().Format(time.RFC3339), "proxies": []any{}, "accounts": accounts}, "", "  ")
	if err != nil {
		return Artifact{}, errors.New("私有文件内容生成失败")
	}
	file, err := os.OpenFile(filepath.Join(s.artifactDirectory, filename), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Artifact{}, errors.New("私有文件创建失败")
	}
	_, writeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := file.Close()
	if errors.Join(writeErr, syncErr, closeErr) != nil {
		_ = os.Remove(filepath.Join(s.artifactDirectory, filename))
		return Artifact{}, errors.New("私有文件保存失败")
	}
	document, _ := json.Marshal(artifactRecord{Artifact: artifact, Owner: owner, RunID: input.ID, File: filename, RunRevision: input.Revision})
	if _, err = s.private.SaveWorkbenchDocument(ctx, "artifact:"+owner+":"+input.ID+":"+artifact.ID, 0, document); err != nil {
		_ = os.Remove(filepath.Join(s.artifactDirectory, filename))
		return Artifact{}, errors.New("私有文件索引保存失败")
	}
	return artifact, nil
}
func (s *Service) deleteArtifacts(ctx context.Context, owner, id string) error {
	rows, err := s.private.WorkbenchDocuments(ctx, "artifact:"+owner+":"+id+":", 501)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var value artifactRecord
		if json.Unmarshal(row.Payload, &value) != nil || value.Owner != owner || value.RunID != id {
			return errors.New("私有文件索引无效")
		}
		if filepath.Base(value.File) != value.File || value.File != value.ID+".json" {
			return errors.New("私有文件名称无效")
		}
		if s.artifactDirectory != "" {
			err = os.Remove(filepath.Join(s.artifactDirectory, value.File))
			if err != nil && !os.IsNotExist(err) {
				return errors.New("私有文件清理失败")
			}
		}
		if err = s.private.DeleteWorkbenchDocument(ctx, row.ID, row.Revision); err != nil {
			return err
		}
	}
	return nil
}
