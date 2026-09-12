package browserlogin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}
type Manager struct {
	factory Factory
	tasks   TaskStore
	runner  taskrunner.Runner
	mu      sync.Mutex
	items   map[string]*session
}
type session struct {
	mu      sync.Mutex
	op      sync.Mutex
	owner   string
	view    View
	browser Browser
	cancel  context.CancelFunc
	finish  chan struct{}
	expires time.Time
	done    bool
}

func New(factory Factory, tasks TaskStore, runner taskrunner.Runner) *Manager {
	return &Manager{factory: factory, tasks: tasks, runner: runner, items: map[string]*session{}}
}
func (m *Manager) Start(ctx context.Context, owner string, record configstore.AuthRecord, commit Commit) (View, error) {
	if owner == "" || commit == nil {
		return View{}, errors.New("浏览器验证缺少有效会话")
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return View{}, err
	}
	id := hex.EncodeToString(raw)
	now := time.Now().UTC()
	value := View{ID: id, TaskID: id, Host: record.Host, Status: "starting", Message: "正在启动验证浏览器", ExpiresAt: now.Add(Lifetime).Format(time.RFC3339Nano), Width: Width, Height: Height}
	s := &session{owner: owner, view: value, finish: make(chan struct{}, 1), expires: now.Add(Lifetime)}
	m.mu.Lock()
	for key, item := range m.items {
		item.mu.Lock()
		active := !item.done
		item.mu.Unlock()
		if now.After(item.expires) && !active {
			delete(m.items, key)
			continue
		}
		if active {
			m.mu.Unlock()
			return View{}, errors.New("已有浏览器验证正在进行，请先完成或关闭现有会话")
		}
	}
	m.items[id] = s
	m.mu.Unlock()
	task := taskstore.Task{ID: id, Skill: "sub2api-upstream-auth", Operation: "browser-login", Status: "queued", Message: value.Message, Result: map[string]any{"host": record.Host}, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}
	if err := m.tasks.Save(ctx, task); err != nil {
		m.remove(id)
		return View{}, err
	}
	if err := taskrunner.GoTask(m.runner, id, func(parent context.Context) { m.execute(parent, s, record, task, commit) }); err != nil {
		m.remove(id)
		taskstore.PersistLaunchFailure(m.tasks, task, err)
		return View{}, err
	}
	return value, nil
}
func (m *Manager) remove(id string) { m.mu.Lock(); delete(m.items, id); m.mu.Unlock() }
func (m *Manager) execute(parent context.Context, s *session, record configstore.AuthRecord, task taskstore.Task, commit Commit) {
	defer func() { s.mu.Lock(); s.done = true; s.mu.Unlock() }()
	ctx, cancel := context.WithDeadline(parent, s.expires)
	defer cancel()
	s.mu.Lock()
	s.cancel = cancel
	cancelled := s.view.Status == "cancelled"
	s.mu.Unlock()
	if cancelled {
		cancel()
	}
	task.Status = "running"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	err := m.tasks.Save(ctx, task)
	if err == nil {
		var b Browser
		b, err = m.factory.Open(ctx, record)
		if err == nil {
			defer b.Close()
			s.mu.Lock()
			s.browser = b
			if s.view.Status != "cancelled" {
				s.view.Status = "waiting"
				s.view.Message = "请在下方上游页面完成验证并登录"
			}
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-s.finish:
				s.op.Lock()
				var credentials configstore.AuthRecord
				credentials, err = b.Credentials(ctx)
				if err == nil {
					err = commit(ctx, credentials)
				}
				s.op.Unlock()
			}
		}
	}
	s.mu.Lock()
	switch {
	case err == nil:
		s.view.Status = "succeeded"
		s.view.Message = "鉴权已复核并保存"
		task.Status = "succeeded"
		task.Result["credentials_persisted"] = true
	case errors.Is(err, context.DeadlineExceeded):
		s.view.Status = "expired"
		s.view.Message = "浏览器验证已超时，请重新打开"
		task.Status = "failed"
	case errors.Is(err, context.Canceled):
		s.view.Status = "cancelled"
		s.view.Message = "浏览器验证已关闭"
		task.Status = "cancelled"
	default:
		s.view.Status = "failed"
		value := []rune(redact.Secrets(err.Error()))
		if len(value) > 300 {
			value = value[:300]
		}
		s.view.Message = "浏览器验证失败：" + string(value)
		task.Status = "failed"
	}
	task.Message = s.view.Message
	task.Progress = 100
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.browser = nil
	s.mu.Unlock()
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer saveCancel()
	if saveErr := m.tasks.Save(saveCtx, task); saveErr != nil {
		slog.Error("浏览器验证任务结果保存失败", "task_id", task.ID)
		s.mu.Lock()
		s.view.Status = "failed"
		s.view.Message = "浏览器验证已结束，但任务结果保存失败，请检查上游状态"
		s.mu.Unlock()
	}
}
func (m *Manager) get(owner, id string) (*session, error) {
	m.mu.Lock()
	s := m.items[id]
	m.mu.Unlock()
	if s == nil || s.owner != owner || time.Now().After(s.expires) {
		return nil, ErrSession
	}
	return s, nil
}
func (m *Manager) Read(ctx context.Context, owner, id string) (View, error) {
	s, err := m.get(owner, id)
	if err != nil {
		return View{}, err
	}
	s.op.Lock()
	defer s.op.Unlock()
	s.mu.Lock()
	v, b := s.view, s.browser
	s.mu.Unlock()
	if v.Status == "waiting" && b != nil {
		shot, err := b.Screenshot(ctx)
		if err != nil {
			return View{}, errors.New("浏览器画面读取失败，请重试或重新打开验证")
		}
		v.Image = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(shot)
	}
	return v, nil
}
func (m *Manager) Input(ctx context.Context, owner, id string, input Input) error {
	if err := input.Validate(); err != nil {
		return err
	}
	s, err := m.get(owner, id)
	if err != nil {
		return err
	}
	s.op.Lock()
	defer s.op.Unlock()
	s.mu.Lock()
	b, ready := s.browser, s.view.Status == "waiting"
	s.mu.Unlock()
	if b == nil || !ready {
		return errors.New("浏览器尚未就绪或已结束")
	}
	if err := b.Input(ctx, input); err != nil {
		return errors.New("浏览器操作未完成，请重新读取画面后再操作")
	}
	return nil
}
func (m *Manager) Finish(owner, id string) error {
	s, err := m.get(owner, id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.view.Status != "waiting" {
		return errors.New("请等待浏览器就绪后再提交")
	}
	s.view.Status = "verifying"
	s.view.Message = "正在复核登录凭据"
	s.finish <- struct{}{}
	return nil
}
func (m *Manager) Cancel(owner, id string) error {
	s, err := m.get(owner, id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.view.Status == "starting" || s.view.Status == "waiting" || s.view.Status == "verifying" {
		s.view.Status = "cancelled"
		s.view.Message = "正在关闭浏览器验证"
		if s.cancel != nil {
			s.cancel()
		}
	}
	return nil
}
