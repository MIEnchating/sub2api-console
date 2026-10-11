package uptimekuma

import (
	"context"
	"fmt"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type TemplateDistributionResult struct {
	MonitorID int64  `json:"monitor_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

func (s *TaskService) DistributeTemplate(ctx context.Context, id string, input TemplateDistributionInput) (taskstore.Task, error) {
	if !templateIDPattern.MatchString(id) || input.Revision < 1 || len(input.Scope) != 64 {
		return taskstore.Task{}, failure("kuma_invalid_distribution", "请先读取参数下发影响范围，再确认下发", 422)
	}
	return s.enqueueProgress(ctx, "uptime-kuma-template-distribute", func(ctx context.Context, report func(int, string, map[string]any) error) (map[string]any, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		result := map[string]any{"template_id": id, "template_revision": input.Revision, "remote_write": false}
		preview, err := s.Service.PreviewTemplateDistribution(ctx, id, input.Revision)
		if err != nil {
			return result, err
		}
		if preview.Scope != input.Scope {
			return result, distributionConflict()
		}
		items := make([]TemplateDistributionResult, len(preview.Items))
		eligible := 0
		for i, item := range preview.Items {
			items[i] = TemplateDistributionResult{MonitorID: item.MonitorID, Name: item.Name, Status: "not_run", Message: "尚未下发"}
			if item.Reason != "" {
				items[i].Status = "skipped"
				items[i].Message = item.Reason
			} else {
				eligible++
			}
		}
		result["items"] = items
		result["total"] = eligible
		result["completed"] = 0
		if eligible == 0 {
			return result, failure("kuma_distribution_empty", "没有可下发的关联监控项，请检查模板绑定和监控类型", 409)
		}
		completed := 0
		for i, item := range preview.Items {
			if item.Reason != "" {
				continue
			}
			if err := ctx.Err(); err != nil {
				return result, err
			}
			if err := report(completed*100/eligible, fmt.Sprintf("正在下发参数（%d/%d）", completed, eligible), result); err != nil {
				return result, failure("kuma_distribution_progress_failed", "任务进度保存失败，已停止后续下发，请核对已完成项目", 500)
			}
			written, err := s.Service.distributeTemplateMonitor(ctx, preview, item)
			if written {
				result["remote_write"] = true
			}
			if err != nil {
				items[i].Status = "failed"
				items[i].Message = PublicError(err).Message
				return result, failure("kuma_distribution_stopped", fmt.Sprintf("已完成 %d/%d 项；下发已停止：%s。请刷新核对后重新预览，未执行项目不会自动重试", completed, eligible, PublicError(err).Message), 409)
			}
			items[i].Status = "succeeded"
			items[i].Message = "参数已下发"
			completed++
			result["completed"] = completed
		}
		return result, nil
	})
}
