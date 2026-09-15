package accountworkbench

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) QueryHistory(ctx context.Context, input taskstore.HistoryQuery) (taskstore.HistoryPage, error) {
	store, ok := s.tasks.(interface {
		QueryBySkill(context.Context, string, taskstore.HistoryQuery) (taskstore.HistoryPage, error)
	})
	if !ok {
		return taskstore.HistoryPage{}, errors.New("历史查询服务尚未就绪")
	}
	return store.QueryBySkill(ctx, Skill, input)
}

func (s *Service) ActiveHistory(ctx context.Context) ([]taskstore.Task, error) {
	store, ok := s.tasks.(interface {
		ActiveBySkill(context.Context, string) ([]taskstore.Task, error)
	})
	if !ok {
		return nil, errors.New("活动任务查询服务尚未就绪")
	}
	return store.ActiveBySkill(ctx, Skill)
}
