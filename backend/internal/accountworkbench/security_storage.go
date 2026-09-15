package accountworkbench

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

var ErrSecurityStorage = errors.New("账号安全结果私有目录不可用，请检查目录权限后重试")

type securityStorage struct {
	mu        sync.Mutex
	directory *os.File
	closed    bool
}

type privateSecurityArtifact struct {
	SourceOAuthBatchID string      `json:"source_oauth_batch_id,omitempty"`
	SourceIndex        *int        `json:"source_index,omitempty"`
	SourceCheckpointID string      `json:"source_checkpoint_id,omitempty"`
	Scope              ExportScope `json:"scope"`
	SourceOAuthID      string      `json:"source_oauth_id,omitempty"`
	WorkspaceID        string      `json:"workspace_id,omitempty"`
	TargetFingerprint  string      `json:"target_fingerprint,omitempty"`
	Version            int         `json:"version"`
	AccountID          string      `json:"account_id,omitempty"`
	Email              string      `json:"email"`
	UserID             string      `json:"user_id"`
	Operation          string      `json:"operation"`
	CreatedAt          string      `json:"created_at"`
	Password           string      `json:"password,omitempty"`
	Secret             string      `json:"secret,omitempty"`
	SessionID          string      `json:"session_id,omitempty"`
}

// Credential artifacts are intentionally private, durable operator results.
// Their contents and filesystem paths never enter a view, task or screenshot.
func (s *Service) UseSecurityDirectory(directory string) error {
	file, err := openPrivateExportDirectory(directory)
	if err != nil {
		return ErrSecurityStorage
	}
	s.securityMu.Lock()
	defer s.securityMu.Unlock()
	if s.securityStorage != nil || len(s.securitySessions) != 0 {
		_ = file.Close()
		return errors.New("安全结果目录已配置，不能在运行期间替换")
	}
	s.securityStorage = &securityStorage{directory: file}
	return nil
}

func (s *Service) CloseSecurity() error {
	if err := s.closeSecurityBatches(); err != nil {
		return err
	}
	s.securityMu.Lock()
	values := make([]*securitySession, 0, len(s.securitySessions))
	for _, value := range s.securitySessions {
		values = append(values, value)
	}
	storage := s.securityStorage
	s.securityMu.Unlock()
	for _, value := range values {
		s.removeSecurity(value.owner, value.view.ID)
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for _, value := range values {
		select {
		case <-value.done:
		case <-deadline.C:
			return errors.New("安全任务尚未完成清理，保留私有目录句柄等待进程退出")
		}
	}
	if storage == nil {
		return nil
	}
	storage.mu.Lock()
	defer storage.mu.Unlock()
	if storage.closed {
		return nil
	}
	storage.closed = true
	return storage.directory.Close()
}

func (s *securityStorage) available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

func (s *securityStorage) write(name string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSecurityStorage
	}
	fd, err := unix.Openat(int(s.directory.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ErrSecurityStorage
	}
	file := os.NewFile(uintptr(fd), name)
	err = json.NewEncoder(file).Encode(value)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = s.directory.Sync()
	}
	if err != nil {
		_ = unix.Unlinkat(int(s.directory.Fd()), name, 0)
		return ErrSecurityStorage
	}
	return nil
}

func (v *securitySession) saveSecret(secret, sessionID string) error {
	artifact := privateSecurityArtifact{Scope: v.view.Scope, SourceOAuthID: v.view.SourceOAuthID, WorkspaceID: v.workspaceID, TargetFingerprint: workbenchScopeFingerprint(v.view.Scope, v.target), Version: 1, AccountID: v.view.AccountID, Email: v.expected.Email, UserID: v.expected.UserID, Operation: v.view.Operation, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Secret: secret, SessionID: sessionID}
	artifact.SourceCheckpointID = v.view.SourceCheckpointID
	artifact.SourceOAuthBatchID, artifact.SourceIndex = v.view.SourceOAuthBatchID, v.view.SourceIndex
	if v.view.Operation == "password" {
		artifact.Password = v.password
	}
	if err := v.storage.write(v.view.ID+".json", artifact); err != nil {
		return err
	}
	v.artifactID = v.view.ID
	return nil
}

func (v *securitySession) saveSecurityOutcome(status string) error {
	if v.artifactID == "" {
		return nil
	}
	return v.storage.write(v.artifactID+".result.json", struct {
		Status    string `json:"status"`
		UpdatedAt string `json:"updated_at"`
	}{Status: status, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
}
