package taskrunner

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrStopped  = errors.New("后台任务执行器已停止")
	ErrNilTask  = errors.New("后台任务不能为空")
	ErrCapacity = errors.New("后台任务并发容量已满")
)

type Runner interface {
	Go(func(context.Context)) error
}

type Group struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	stopped bool
	active  int
	slots   chan struct{}
	done    chan struct{}
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
	group := &Group{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	if maxActive > 0 {
		group.slots = make(chan struct{}, maxActive)
	}
	return group
}

func (g *Group) Go(run func(context.Context)) error {
	if run == nil {
		return ErrNilTask
	}
	g.mu.Lock()
	if g.stopped || g.ctx.Err() != nil {
		g.mu.Unlock()
		return ErrStopped
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
	g.mu.Unlock()
	go func() {
		defer func() {
			if g.slots != nil {
				<-g.slots
			}
			g.finish()
		}()
		run(g.ctx)
	}()
	return nil
}

func (g *Group) finish() {
	g.mu.Lock()
	g.active--
	if g.stopped && g.active == 0 {
		close(g.done)
	}
	g.mu.Unlock()
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
