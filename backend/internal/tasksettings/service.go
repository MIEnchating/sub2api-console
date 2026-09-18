package tasksettings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

var ErrConflict = errors.New("任务并发设置已更新，请刷新后重试")
var ErrInvalid = errors.New("任务并发参数无效")

type Repository interface {
	TaskLimits(context.Context) (string, error)
	SaveTaskLimits(context.Context, string, string) error
}

type Input struct {
	Limits        map[string]int `json:"limits"`
	QueueCapacity int            `json:"queue_capacity"`
	Version       string         `json:"version"`
}
type Pool struct {
	ID string `json:"id"`
	taskrunner.Snapshot
}
type State struct {
	Pools         []Pool `json:"pools"`
	QueueCapacity int    `json:"queue_capacity"`
	Version       string `json:"version"`
}
type Service struct {
	mu     sync.Mutex
	store  Repository
	groups map[string]*taskrunner.Group
	raw    string
}

func New(ctx context.Context, store Repository, groups map[string]*taskrunner.Group) (*Service, error) {
	raw, err := store.TaskLimits(ctx)
	if err != nil {
		return nil, err
	}
	s := &Service{store: store, groups: groups, raw: raw}
	if raw != "" {
		var saved Input
		if err := json.Unmarshal([]byte(raw), &saved); err != nil {
			return nil, fmt.Errorf("任务并发设置读取失败: %w", err)
		}
		if err := s.validate(saved); err != nil {
			return nil, err
		}
		s.apply(saved)
	}
	return s, nil
}

func (s *Service) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}
func (s *Service) snapshot() State {
	state := State{Pools: make([]Pool, 0, len(s.groups)), Version: version(s.raw)}
	for id, g := range s.groups {
		snapshot := g.Snapshot()
		state.Pools = append(state.Pools, Pool{ID: id, Snapshot: snapshot})
		state.QueueCapacity = snapshot.QueueCapacity
	}
	sort.Slice(state.Pools, func(i, j int) bool { return state.Pools[i].ID < state.Pools[j].ID })
	return state
}
func (s *Service) Update(ctx context.Context, input Input) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.Version != version(s.raw) {
		return State{}, ErrConflict
	}
	if err := s.validate(input); err != nil {
		return State{}, err
	}
	if len(input.Limits) != len(s.groups) {
		return State{}, fmt.Errorf("%w：请提交全部模块", ErrInvalid)
	}
	input.Version = ""
	raw, err := json.Marshal(input)
	if err != nil {
		return State{}, err
	}
	if err := s.store.SaveTaskLimits(ctx, s.raw, string(raw)); err != nil {
		return State{}, err
	}
	s.raw = string(raw)
	s.apply(input)
	return s.snapshot(), nil
}
func (s *Service) validate(input Input) error {
	if input.QueueCapacity < 1 || input.QueueCapacity > 10000 {
		return fmt.Errorf("%w：等待队列容量须为 1–10000", ErrInvalid)
	}
	for id, limit := range input.Limits {
		if s.groups[id] == nil || limit < 1 || limit > 10000 {
			return fmt.Errorf("%w：模块 %s 的并发须为 1–10000", ErrInvalid, id)
		}
	}
	return nil
}
func (s *Service) apply(input Input) {
	for id, g := range s.groups {
		limit := input.Limits[id]
		if limit == 0 {
			limit = g.Snapshot().Limit
		}
		g.Configure(limit, input.QueueCapacity)
	}
}
func version(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}
