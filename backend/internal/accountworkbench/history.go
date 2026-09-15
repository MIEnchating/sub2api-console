package accountworkbench

import (
	"context"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type HistoryActionInput struct {
	Items     []taskstore.HistorySelection `json:"items"`
	Confirmed bool                         `json:"confirmed"`
}
type HistoryCancelItem struct {
	ID        string `json:"id"`
	Cancelled bool   `json:"cancelled"`
	Message   string `json:"message"`
}

func (s *Service) DeleteHistory(ctx context.Context, input HistoryActionInput) error {
	if !input.Confirmed {
		return errors.New("请先确认要删除的处理记录")
	}
	store, ok := s.tasks.(interface {
		DeleteTerminalBySkill(context.Context, string, []taskstore.HistorySelection) error
	})
	if !ok {
		return errors.New("处理记录删除服务尚未就绪")
	}
	return store.DeleteTerminalBySkill(ctx, Skill, input.Items)
}

func (s *Service) CancelHistory(ctx context.Context, input HistoryActionInput) ([]HistoryCancelItem, error) {
	if !input.Confirmed || len(input.Items) == 0 || len(input.Items) > 10000 {
		return nil, errors.New("请先确认 1～10000 个正在执行的工作台任务")
	}
	store, ok := s.tasks.(interface {
		Get(context.Context, string) (taskstore.Task, error)
	})
	if !ok {
		return nil, errors.New("处理任务查询服务尚未就绪")
	}
	runner, ok := s.runner.(interface{ CancelTask(string) bool })
	if !ok {
		return nil, errors.New("处理任务取消服务尚未就绪")
	}
	seen := make(map[string]bool, len(input.Items))
	for _, item := range input.Items {
		if strings.TrimSpace(item.ID) == "" || len(item.ID) > 255 || seen[item.ID] {
			return nil, errors.New("处理任务选择无效")
		}
		seen[item.ID] = true
		task, err := store.Get(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		if task.Skill != Skill {
			return nil, errors.New("选中的任务不属于账号工作台")
		}
	}
	result := make([]HistoryCancelItem, 0, len(input.Items))
	for _, item := range input.Items {
		row := HistoryCancelItem{ID: item.ID, Message: "任务已经结束或已不在当前进程运行，请刷新记录"}
		task, err := store.Get(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		if task.Status == "queued" || task.Status == "running" || task.Status == "waiting_input" {
			oauthCancelled, err := s.CancelOAuthTask(item.ID)
			if err != nil {
				return nil, err
			}
			row.Cancelled = runner.CancelTask(item.ID) || oauthCancelled
			if row.Cancelled {
				row.Message = "已请求取消，请等待任务保存已完成结果"
			}
		}
		result = append(result, row)
	}
	return result, nil
}
