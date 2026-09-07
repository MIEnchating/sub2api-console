package taskrunner

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
)

var (
	ErrStopped                     = errors.New("后台任务执行器已停止")
	ErrNilTask                     = errors.New("后台任务不能为空")
	ErrCapacity                    = errors.New("后台任务并发容量已满")
	ErrTaskID                      = errors.New("后台任务 ID 不能为空")
	ErrDuplicateTask               = errors.New("同一后台任务已在运行")
	ErrTaskCancellationUnsupported = errors.New("后台任务执行器不支持按任务取消")
)

type Runner interface {
	Go(func(context.Context)) error
}

type TaskRunner interface {
	Runner
	GoTask(string, func(context.Context)) error
	CancelTask(string) bool
}

type Group struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	stopped bool
	active  int
	slots   chan struct{}
	done    chan struct{}
	tasks   map[string]context.CancelFunc
}

func New(parent context.Context) *Group {
	return newGroup(parent, 0)
}

func NewBounded(parent context.Context, maxActive int) *Group {
	if maxActive < 1 {
		maxActive = 1
	}
	return newGroup(parent, maxActive)
}

func newGroup(parent context.Context, maxActive int) *Group {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	group := &Group{ctx: ctx, cancel: cancel, done: make(chan struct{}), tasks: map[string]context.CancelFunc{}}
	if maxActive > 0 {
		group.slots = make(chan struct{}, maxActive)
	}
	return group
}

func (g *Group) Go(run func(context.Context)) error {
	return g.start("", run)
}

func (g *Group) GoTask(taskID string, run func(context.Context)) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ErrTaskID
	}
	return g.start(taskID, run)
}

func (g *Group) start(taskID string, run func(context.Context)) error {
	if run == nil {
		return ErrNilTask
	}
	g.mu.Lock()
	if g.stopped || g.ctx.Err() != nil {
		g.mu.Unlock()
		return ErrStopped
	}
	if taskID != "" {
		if _, exists := g.tasks[taskID]; exists {
			g.mu.Unlock()
			return ErrDuplicateTask
		}
	}
	if g.slots != nil {
		select {
		case g.slots <- struct{}{}:
		default:
			g.mu.Unlock()
			return ErrCapacity
		}
	}
	g.active++
	runContext := g.ctx
	var cancel context.CancelFunc
	if taskID != "" {
		runContext, cancel = context.WithCancel(g.ctx)
		runContext = taskcontext.WithID(runContext, taskID)
		g.tasks[taskID] = cancel
	}
	g.mu.Unlock()
	go func() {
		defer func() {
			if cancel != nil {
				cancel()
			}
			if g.slots != nil {
				<-g.slots
			}
			g.finish(taskID)
		}()
		run(runContext)
	}()
	return nil
}

func (g *Group) finish(taskID string) {
	g.mu.Lock()
	if taskID != "" {
		delete(g.tasks, taskID)
	}
	g.active--
	if g.stopped && g.active == 0 {
		close(g.done)
	}
	g.mu.Unlock()
}

func (g *Group) CancelTask(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	g.mu.Lock()
	cancel, found := g.tasks[taskID]
	g.mu.Unlock()
	if !found {
		return false
	}
	cancel()
	return true
}

func (g *Group) Cancel() {
	g.mu.Lock()
	if !g.stopped {
		g.stopped = true
		g.cancel()
		if g.active == 0 {
			close(g.done)
		}
	}
	g.mu.Unlock()
}

func (g *Group) Shutdown(ctx context.Context) error {
	g.Cancel()
	select {
	case <-g.done:
		return nil
	case <-ctx.Done():
		select {
		case <-g.done:
			return nil
		default:
			return ctx.Err()
		}
	}
}

func Go(runner Runner, run func(context.Context)) error {
	if run == nil {
		return ErrNilTask
	}
	if runner != nil {
		return runner.Go(run)
	}
	go run(context.Background())
	return nil
}

func GoTask(runner Runner, taskID string, run func(context.Context)) error {
	if run == nil {
		return ErrNilTask
	}
	if strings.TrimSpace(taskID) == "" {
		return ErrTaskID
	}
	if keyed, ok := runner.(TaskRunner); ok {
		return keyed.GoTask(taskID, run)
	}
	if runner != nil {
		return ErrTaskCancellationUnsupported
	}
	go run(taskcontext.WithID(context.Background(), taskID))
	return nil
}
