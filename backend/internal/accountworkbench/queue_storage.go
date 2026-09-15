package accountworkbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type queueStore interface {
	WorkbenchQueues(context.Context, string, string) ([]configstore.WorkbenchQueue, error)
	WorkbenchQueue(context.Context, string, string, string) (configstore.WorkbenchQueue, error)
	SaveWorkbenchQueue(context.Context, configstore.WorkbenchQueue) (configstore.WorkbenchQueue, error)
	DeleteWorkbenchQueue(context.Context, string, string, string, int64) error
	PurgeExpiredWorkbenchQueues(context.Context, time.Time) error
}

type privateQueueItem struct {
	Item        InputItem      `json:"item"`
	Credentials map[string]any `json:"credentials"`
}

type oauthQueuePayload struct {
	Checkpoints map[int]oauthQueueCheckpoint `json:"checkpoints,omitempty"`
	Version     int                          `json:"version"`
	View        OAuthBatchView               `json:"view"`
	Inputs      []OAuthLoginInput            `json:"inputs"`
	Results     []privateQueueItem           `json:"results"`
}

type oauthQueueCheckpoint struct {
	ID             string `json:"id"`
	SourceTaskID   string `json:"source_task_id"`
	ParentID       string `json:"parent_id"`
	Revision       int64  `json:"revision"`
	WorkerRevision int64  `json:"worker_revision"`
}

func queueItems(items []InputItem) []privateQueueItem {
	result := make([]privateQueueItem, len(items))
	for i, item := range items {
		result[i] = privateQueueItem{Item: item, Credentials: item.Credentials}
	}
	return result
}

func restoreQueueItems(items []privateQueueItem, count int) ([]InputItem, error) {
	result := make([]InputItem, 0, len(items))
	seen := map[int]bool{}
	for _, item := range items {
		if item.Item.Index < 0 || item.Item.Index >= count || seen[item.Item.Index] {
			return nil, configstore.ErrWorkbenchQueue
		}
		raw, err := json.Marshal(map[string]any{"credentials": item.Credentials, "email": item.Item.Email, "name": item.Item.Name})
		if err != nil {
			return nil, configstore.ErrWorkbenchQueue
		}
		parsed, failures := Parse(string(raw))
		if len(failures) > 0 || len(parsed) != 1 || stringValue(parsed[0].Credentials["access_token"]) == "" || IdentityKey(parsed[0]) == "" {
			return nil, configstore.ErrWorkbenchQueue
		}
		parsed[0].Index = item.Item.Index
		seen[item.Item.Index] = true
		result = append(result, parsed[0])
	}
	return result, nil
}

func (s *Service) queueStore() (queueStore, error) {
	store, ok := s.private.(queueStore)
	if !ok {
		return nil, errors.New("私有批量恢复存储尚未就绪")
	}
	return store, nil
}

// Call with job.mu held. The private receipt precedes the public task update.
func (s *Service) persistOAuthQueue(ctx context.Context, job *oauthBatch, status string) error {
	if job.queueFrozen {
		return nil
	}
	if job.parentQueuePersist != nil {
		return job.parentQueuePersist(ctx, job, status)
	}
	if job.queue == nil {
		return nil
	}
	store, err := s.queueStore()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(oauthQueuePayload{Version: 1, View: job.view, Inputs: job.inputs, Results: queueItems(job.results), Checkpoints: job.checkpoints})
	if err != nil {
		return errors.New("批量授权恢复资料无法编码")
	}
	record := *job.queue
	record.Status, record.Payload = status, payload
	saved, err := saveQueueRecord(ctx, store, record)
	if err != nil {
		return errors.New("批量授权恢复资料未可靠保存，已停止后续授权")
	}
	job.queue = &saved
	return nil
}

// A local commit can outlive its caller. Readback recognizes only this exact CAS
// successor; it never retries an uncertain write or adopts another runner.
func saveQueueRecord(ctx context.Context, store queueStore, record configstore.WorkbenchQueue) (configstore.WorkbenchQueue, error) {
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	saved, err := store.SaveWorkbenchQueue(persist, record)
	if err == nil {
		return saved, nil
	}
	read, cancelRead := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelRead()
	current, readErr := store.WorkbenchQueue(read, record.Owner, record.Target, record.ID)
	if readErr != nil || current.Revision != record.Revision+1 || current.TaskID != record.TaskID || current.Kind != record.Kind || current.Status != record.Status || current.Owner != record.Owner || current.Target != record.Target || current.ID != record.ID || !sameQueueTime(current.CreatedAt, record.CreatedAt) || !sameQueueTime(current.ExpiresAt, record.ExpiresAt) || !bytes.Equal(current.Payload, record.Payload) {
		return configstore.WorkbenchQueue{}, err
	}
	current.Payload = nil
	return current, nil
}

func sameQueueTime(a, b string) bool {
	left, leftErr := time.Parse(time.RFC3339Nano, a)
	right, rightErr := time.Parse(time.RFC3339Nano, b)
	return leftErr == nil && rightErr == nil && left.Equal(right)
}

func (s *Service) deleteOAuthQueue(job *oauthBatch) error {
	if job.queue == nil {
		return nil
	}
	store, err := s.queueStore()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	record := job.queue
	if err := store.DeleteWorkbenchQueue(ctx, record.Owner, record.Target, record.ID, record.Revision); err != nil {
		return err
	}
	job.queueFrozen = true
	return nil
}
