package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

var ErrRun = errors.New("处理记录不存在、已过期或不属于当前会话")

type RunItem struct {
	InputItem
	Status        string         `json:"status"`
	Message       string         `json:"message"`
	AccountID     string         `json:"account_id,omitempty"`
	ImportAction  string         `json:"import_action,omitempty"`
	TemplateName  string         `json:"template_name"`
	Check         map[string]any `json:"check,omitempty"`
	ManualEnabled bool           `json:"manual_enabled,omitempty"`
	BrowserReady  bool           `json:"browser_ready,omitempty"`
}
type Run struct {
	ID             string    `json:"id"`
	Revision       int64     `json:"revision"`
	TaskID         string    `json:"task_id"`
	Status         string    `json:"status"`
	Action         string    `json:"action"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	DuplicateCount int       `json:"duplicate_count"`
	Items          []RunItem `json:"items"`
}
type privateRun struct {
	Scope            string                     `json:"scope"`
	Public           Run                        `json:"public"`
	Owner            string                     `json:"owner"`
	Target           configstore.TargetSettings `json:"target"`
	Settings         PreviewInput               `json:"settings"`
	Template         *Template                  `json:"template"`
	TemplateRevision int64                      `json:"template_revision"`
	Items            []storedInput              `json:"items"`
	Exports          []map[string]any           `json:"exports"`
	AccountVersions  map[string]string          `json:"account_versions"`
	// A durable phase is recorded before every one-use or uncertain write.
	Phases         map[string]string `json:"phases"`
	TaskIDs        []string          `json:"task_ids"`
	ManualIDs      []string          `json:"manual_ids,omitempty"`
	SecretsCleared bool              `json:"secrets_cleared,omitempty"`
	Automatic      bool              `json:"automatic,omitempty"`
}

func (s *Service) Runs(ctx context.Context, owner string) ([]Run, error) {
	if err := s.checkOwner(ctx, owner); err != nil {
		return nil, err
	}
	result := []Run{}
	err := s.documents(ctx, "run:"+owner+":", func(record configstore.WorkbenchDocumentRecord) error {
		value, err := decodeRun(record.Payload)
		if err != nil {
			return err
		}
		if value.Owner != owner {
			return ErrRun
		}
		if value.Public.Action == "maintenance" {
			return nil
		}
		value.Public.Revision = record.Revision
		result = append(result, value.Public)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(result, func(a, b Run) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return result, nil
}

func (s *Service) Run(ctx context.Context, owner, id string) (Run, error) {
	value, err := s.readRun(ctx, owner, id)
	return value.Public, err
}
func runKey(owner, id string) string { return "run:" + owner + ":" + id }
func (s *Service) readRun(ctx context.Context, owner, id string) (privateRun, error) {
	if err := s.checkOwner(ctx, owner); err != nil {
		return privateRun{}, err
	}
	if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" {
		return privateRun{}, ErrRun
	}
	raw, revision, err := s.private.WorkbenchDocument(ctx, runKey(owner, id))
	if err != nil {
		return privateRun{}, err
	}
	if len(raw) == 0 {
		return privateRun{}, ErrRun
	}
	value, err := decodeRun(raw)
	if err != nil {
		return privateRun{}, err
	}
	if value.Owner != owner || value.Public.ID != id || value.Public.Action == "maintenance" {
		return privateRun{}, ErrRun
	}
	value.Public.Revision = revision
	return value, nil
}
func decodeRun(raw json.RawMessage) (privateRun, error) {
	var value privateRun
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return value, errors.New("处理记录无法读取，请重新输入账号资料")
	}
	return value, nil
}
func (s *Service) saveRun(ctx context.Context, value *privateRun) error {
	value.Public.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(value)
	if err != nil {
		return errors.New("处理记录无法保存")
	}
	revision, err := s.private.SaveWorkbenchDocument(ctx, runKey(value.Owner, value.Public.ID), value.Public.Revision, raw)
	if err == nil {
		value.Public.Revision = revision
	}
	return err
}
