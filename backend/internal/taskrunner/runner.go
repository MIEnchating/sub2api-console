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

// CompositeCanceller forwards cancellation to every task group that may own a
// task. The console uses separate bounded groups for user operations and live
// workbench tasks, while the API exposes one cancellation endpoint.
type CompositeCanceller struct {
	Groups []TaskRunner
}

func (c CompositeCanceller) CancelTask(taskID string) bool {
	found := false
	for _, group := range c.Groups {
		if group != nil && group.CancelTask(taskID) {
			found = true
		}
	}
	return found
}

type Group struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	stopped bool
	active  int
	running int
	limit   int
	waiting int
	queue   int
	changed chan struct{}
	done    chan struct{}
	tasks   map[string]context.CancelFunc
}

func New(parent context.Context) *Group {
	return newGroup(parent, 0, 0)
}

func NewBounded(parent context.Context, maxActive int) *Group {
	return NewQueued(parent, maxActive, 0)
}

// NewQueued bounds active work and lets a finite number of callers wait for a
// slot. A zero queue retains fail-fast behavior.
func NewQueued(parent context.Context, maxActive, queueCapacity int) *Group {
	if maxActive < 1 {
		maxActive = 1
	}
	return newGroup(parent, maxActive, queueCapacity)
}

func newGroup(parent context.Context, maxActive, queueCapacity int) *Group {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	group := &Group{ctx: ctx, cancel: cancel, done: make(chan struct{}), tasks: map[string]context.CancelFunc{}, limit: maxActive, queue: queueCapacity, changed: make(chan struct{})}
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
	queued := g.limit > 0 && g.running >= g.limit
	if queued {
		if g.waiting >= g.queue {
			g.mu.Unlock()
			return ErrCapacity
		}
		g.waiting++
	}
	g.active++
	if !queued {
		g.running++
	}
	runContext := g.ctx
	var cancel context.CancelFunc
	if taskID != "" {
		runContext, cancel = context.WithCancel(g.ctx)
		runContext = taskcontext.WithID(runContext, taskID)
		g.tasks[taskID] = cancel
	}
	g.mu.Unlock()
	go func() {
		acquired := !queued
		if queued {
			acquired = g.wait(runContext)
		}
		defer func() {
			if cancel != nil {
				cancel()
			}
			g.finish(taskID, acquired)
		}()
		// Even cancelled waiting tasks must finalize their persisted state.
		run(runContext)
	}()
	return nil
}

func (g *Group) wait(ctx context.Context) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	defer func() { g.waiting-- }()
	for {
		if ctx.Err() != nil {
			return false
		}
		if g.limit == 0 || g.running < g.limit {
			g.running++
			return true
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
		}
		g.mu.Lock()
	}
}

func (g *Group) finish(taskID string, acquired bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if taskID != "" {
		delete(g.tasks, taskID)
	}
	g.active--
	if acquired {
		g.running--
	}
	g.notifyLocked()
	if g.stopped && g.active == 0 {
		close(g.done)
	}
}

func (g *Group) notifyLocked() {
	close(g.changed)
	g.changed = make(chan struct{})
}

// Configure changes future dispatch capacity without cancelling running work.
func (g *Group) Configure(limit, queue int) {
	if limit < 1 || queue < 0 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.limit, g.queue = limit, queue
	g.notifyLocked()
}

type Snapshot struct {
	Limit         int `json:"limit"`
	QueueCapacity int `json:"queue_capacity"`
	Running       int `json:"running"`
	Waiting       int `json:"waiting"`
}

func (g *Group) Snapshot() Snapshot {
	g.mu.Lock()
	defer g.mu.Unlock()
	return Snapshot{Limit: g.limit, QueueCapacity: g.queue, Running: g.running, Waiting: g.waiting}
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
