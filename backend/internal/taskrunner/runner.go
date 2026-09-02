package taskrunner

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrStopped = errors.New("后台任务执行器已停止")
	ErrNilTask = errors.New("后台任务不能为空")
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
	done    chan struct{}
}

func New(parent context.Context) *Group {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &Group{ctx: ctx, cancel: cancel, done: make(chan struct{})}
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
	g.active++
	g.mu.Unlock()
	go func() {
		defer g.finish()
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
