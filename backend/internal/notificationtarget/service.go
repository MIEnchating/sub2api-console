package notificationtarget

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

var ErrDiscoveryActive = errors.New("已有 QQBot 目标获取任务正在运行，请先完成或取消")

type Request struct {
	AppID        string
	ClientSecret string
	TargetType   string
}

type Target struct {
	ID         string `json:"target_id"`
	Type       string `json:"target_type"`
	EventType  string `json:"event_type"`
	SourceName string `json:"source_name,omitempty"`
	CapturedAt string `json:"captured_at"`
}

type Listener interface {
	Listen(context.Context, Request, func()) (Target, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type Service struct {
	listener Listener
	tasks    TaskStore
	timeout  time.Duration

	mu           sync.Mutex
	activeTaskID string
	cancel       context.CancelFunc
}

func New(listener Listener, tasks TaskStore) *Service {
	return newService(listener, tasks, 2*time.Minute)
}

func newService(listener Listener, tasks TaskStore, timeout time.Duration) *Service {
	return &Service{listener: listener, tasks: tasks, timeout: timeout}
}

func (s *Service) Enqueue(ctx context.Context, request Request) (taskstore.Task, error) {
	request.AppID = strings.TrimSpace(request.AppID)
	request.ClientSecret = strings.TrimSpace(request.ClientSecret)
	request.TargetType = strings.ToLower(strings.TrimSpace(request.TargetType))
	if request.AppID == "" || request.ClientSecret == "" {
		return taskstore.Task{}, errors.New("请先填写 App ID 和 Client Secret")
	}
	if request.TargetType != "c2c" && request.TargetType != "group" && request.TargetType != "channel" {
		return taskstore.Task{}, errors.New("目标类型只能是私聊、群聊或频道")
	}
	if s.listener == nil || s.tasks == nil {
		return taskstore.Task{}, errors.New("QQBot 目标获取服务尚未就绪")
	}
	id, err := discoveryTaskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "qqbot", Operation: "discover-notification-target", Status: "queued", Progress: 0,
		Message: "QQBot 目标获取任务已创建", Result: map[string]any{"target_type": request.TargetType},
		CreatedAt: now, UpdatedAt: now,
	}
	discoveryContext, cancel := context.WithTimeout(context.Background(), s.timeout)
	s.mu.Lock()
	if s.activeTaskID != "" {
		s.mu.Unlock()
		cancel()
		return taskstore.Task{}, ErrDiscoveryActive
	}
	s.activeTaskID, s.cancel = id, cancel
	s.mu.Unlock()
	if err := s.tasks.Save(ctx, task); err != nil {
		s.clearActive(id)
		cancel()
		return taskstore.Task{}, err
	}
	go s.execute(discoveryContext, cancel, task, request)
	return task, nil
}

func (s *Service) Cancel(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if taskID == "" || s.activeTaskID != taskID || s.cancel == nil {
		return false
	}
	s.cancel()
	return true
}

func (s *Service) execute(ctx context.Context, cancel context.CancelFunc, task taskstore.Task, request Request) {
	defer cancel()
	defer s.clearActive(task.ID)
	task.Status, task.Progress, task.Message = "running", 15, "正在连接 QQBot 事件服务"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistProgress(s.tasks, task)
	ready := false
	target, err := s.listener.Listen(ctx, request, func() {
		if ready {
			return
		}
		ready = true
		task.Status, task.Progress = "waiting_input", 60
		task.Message = discoveryInstruction(request.TargetType)
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	})
	task.Progress = 100
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err == nil {
		task.Status, task.Message = "succeeded", "已获取通知目标并自动填入"
		task.Result = map[string]any{
			"target_id": target.ID, "target_type": target.Type, "event_type": target.EventType,
			"source_name": target.SourceName, "captured_at": target.CapturedAt,
		}
	} else if errors.Is(ctx.Err(), context.Canceled) {
		task.Status, task.Message = "cancelled", "已取消 QQBot 目标获取"
		task.Result = map[string]any{"target_type": request.TargetType, "cancelled": true}
	} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		task.Status, task.Message = "failed", "等待消息超时，请重新连接后再发送消息"
		task.Result = map[string]any{"target_type": request.TargetType, "error": "等待消息超时"}
	} else {
		task.Status, task.Message = "failed", "QQBot 目标获取失败："+safeReason(err)
		task.Result = map[string]any{"target_type": request.TargetType, "error": safeReason(err)}
	}
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) clearActive(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeTaskID == taskID {
		s.activeTaskID = ""
		s.cancel = nil
	}
}

func discoveryInstruction(targetType string) string {
	switch targetType {
	case "group":
		return "已连接，请在目标群里 @机器人并发送任意消息；机器人无需回复"
	case "channel":
		return "已连接，请在目标子频道里 @机器人并发送任意消息；机器人无需回复"
	default:
		return "已连接，请在 QQ 私聊中给机器人发送任意消息；机器人无需回复"
	}
}

func safeReason(err error) string {
	if err == nil || strings.TrimSpace(err.Error()) == "" {
		return "事件服务未返回原因"
	}
	value := strings.TrimSpace(err.Error())
	if len([]rune(value)) > 300 {
		return string([]rune(value)[:300])
	}
	return value
}

func discoveryTaskID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "qqbot-target-" + hex.EncodeToString(buffer), nil
}
