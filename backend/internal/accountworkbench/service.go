// Package accountworkbench implements the GitHub AccountWorkbench product in Go.
package accountworkbench

import (
	"context"
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"net/http"
	"sync"
	"time"
)

const Skill = "account-workbench"

type privateStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
	WorkbenchDocument(context.Context, string) (json.RawMessage, int64, error)
	SaveWorkbenchDocument(context.Context, string, int64, json.RawMessage) (int64, error)
	WorkbenchDocuments(context.Context, string, int) ([]configstore.WorkbenchDocumentRecord, error)
	WorkbenchDocumentPage(context.Context, string, string, int) ([]configstore.WorkbenchDocumentRecord, error)
	DeleteWorkbenchDocument(context.Context, string, int64) error
	ActiveSessionOwner(context.Context, string, time.Time) (bool, error)
}

type Service struct {
	private         privateStore
	groupProtection interface {
		AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error)
	}
	transport         http.RoundTripper
	officialTransport http.RoundTripper
	mailTransport     http.RoundTripper
	tasks             taskRepository
	runner            taskrunner.TaskRunner
	checker           OAuthChecker
	artifactDirectory string
	opMu              sync.Mutex
	activeMu          sync.Mutex
	active            map[string]*activeRun
}

func New(store privateStore) *Service {
	return &Service{private: store, active: map[string]*activeRun{}}
}

func (s *Service) UseAccountGroupProtection(repository interface {
	AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error)
}) {
	s.groupProtection = repository
}

func (s *Service) requireGroupsUnlocked(ctx context.Context, accountID string) error {
	if s.groupProtection == nil {
		return nil
	}
	protection, err := s.groupProtection.AccountMutationProtection(ctx, accountID)
	if err != nil {
		return err
	}
	if protection.GroupsLocked {
		return business.ErrAccountGroupsLocked
	}
	return nil
}

func (s *Service) acquireGroupMutation(ctx context.Context, resources ...string) (context.Context, func() error, error) {
	if s.groupProtection != nil {
		return targetguard.Acquire(ctx, s.groupProtection, resources...)
	}
	return targetguard.Acquire(ctx, s.private, resources...)
}

type taskRepository interface {
	taskstore.Saver
	Get(context.Context, string) (taskstore.Task, error)
	DeleteTerminalBySkill(context.Context, string, []taskstore.HistorySelection) error
}
type OAuthChecker interface {
	CheckOAuthWithProxy(context.Context, string, string, map[string]any, string, int, string) (map[string]any, error)
}

func (s *Service) UseExecution(tasks taskRepository, runner taskrunner.TaskRunner, checker OAuthChecker, artifactDirectory string) {
	s.tasks = tasks
	s.runner = runner
	s.checker = checker
	s.artifactDirectory = artifactDirectory
}

// UseTransport replaces only the HTTP boundary in isolated tests.
func (s *Service) UseTransport(transport http.RoundTripper) { s.transport = transport }

func (s *Service) UseMailTransport(transport http.RoundTripper) { s.mailTransport = transport }

func (s *Service) client(target configstore.TargetSettings) (*adminclient.Client, error) {
	return adminclient.New(adminclient.Config{BaseURL: target.BaseURL, AdminKey: target.AdminKey, Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 1}, s.transport)
}
