package accountworkbench

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) Retry(ctx context.Context, owner string, input RunConfirmation) (Run, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	value, err := s.confirmRun(ctx, owner, input)
	if err != nil {
		return Run{}, err
	}
	s.activeMu.Lock()
	active := s.active[input.ID] != nil
	s.activeMu.Unlock()
	if active {
		return Run{}, errors.New("任务仍在运行，请等待或先取消")
	}
	if err = s.executionAllowed(ctx, &value); err != nil {
		return Run{}, err
	}
	retryable := 0
	for i := range value.Public.Items {
		row := &value.Public.Items[i]
		if row.Status == "completed" || row.Status == "exported" {
			continue
		}
		switch value.Phases[row.ID] {
		case "refreshing", "exchanging":
			continue
		}
		row.Status = "queued"
		row.Message = "等待继续处理"
		retryable++
	}
	if retryable == 0 {
		return Run{}, errors.New("没有可重试项；结果不明的提交需先在官方或站点核对")
	}
	value.ManualIDs = nil
	return s.launch(ctx, &value)
}
func (s *Service) EnableReview(ctx context.Context, owner string, input RunConfirmation, ids []string) (Run, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	value, err := s.confirmRun(ctx, owner, input)
	if err != nil {
		return Run{}, err
	}
	s.activeMu.Lock()
	active := s.active[input.ID] != nil
	s.activeMu.Unlock()
	if active {
		return Run{}, errors.New("任务仍在运行，请等待结束")
	}
	if value.Public.Action != "import" || len(ids) == 0 || len(ids) > len(value.Items) {
		return Run{}, errors.New("请选择检测已完成且已隔离导入的账号")
	}
	if err = s.executionAllowed(ctx, &value); err != nil {
		return Run{}, err
	}
	selected := map[string]bool{}
	for _, id := range ids {
		if selected[id] {
			return Run{}, errors.New("不能重复选择账号")
		}
		selected[id] = true
	}
	found := 0
	for _, row := range value.Public.Items {
		if selected[row.ID] {
			if row.Status != "review" || len(row.Check) == 0 || row.AccountID == "" || (value.Phases[row.ID] != "configured" && value.Phases[row.ID] != "isolated") {
				return Run{}, errors.New("仅允许手动启用已导入、检测未通过且仍保持隔离的账号")
			}
			found++
		}
	}
	if found != len(ids) {
		return Run{}, errors.New("所选账号不属于本批记录")
	}
	value.ManualIDs = append([]string(nil), ids...)
	return s.launch(ctx, &value)
}
func (s *Service) confirmRun(ctx context.Context, owner string, input RunConfirmation) (privateRun, error) {
	value, err := s.readRun(ctx, owner, input.ID)
	if err != nil {
		return value, err
	}
	if value.Public.Revision != input.Revision {
		return value, configstore.ErrWorkbenchVersion
	}
	return value, nil
}
func (s *Service) Cancel(ctx context.Context, owner string, input RunConfirmation) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if _, err := s.confirmRun(ctx, owner, input); err != nil {
		return err
	}
	s.activeMu.Lock()
	active := s.active[input.ID]
	s.activeMu.Unlock()
	if active == nil {
		return nil
	}
	s.runner.CancelTask(active.taskID)
	return nil
}
func (s *Service) Delete(ctx context.Context, owner string, input RunConfirmation) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	value, err := s.confirmRun(ctx, owner, input)
	if err != nil {
		return err
	}
	s.activeMu.Lock()
	active := s.active[input.ID]
	s.activeMu.Unlock()
	if active != nil {
		s.runner.CancelTask(active.taskID)
		select {
		case <-active.done:
		case <-ctx.Done():
			return errors.New("任务正在停止，请等待当前官方请求结束后重新删除")
		}
		value, err = s.readRun(ctx, owner, input.ID)
		if err != nil {
			return err
		}
	}
	return s.deleteRecord(ctx, &value)
}
func (s *Service) deleteRecord(ctx context.Context, value *privateRun) error {
	if len(value.TaskIDs) > 0 && s.tasks == nil {
		return errors.New("工作台任务存储尚未就绪")
	}
	for _, id := range value.TaskIDs {
		task, err := s.tasks.Get(ctx, id)
		if errors.Is(err, taskstore.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err = s.tasks.DeleteTerminalBySkill(ctx, Skill, []taskstore.HistorySelection{{ID: id, UpdatedAt: task.UpdatedAt}}); err != nil {
			return err
		}
	}
	if err := s.deleteArtifacts(ctx, value.Owner, value.Public.ID); err != nil {
		return err
	}
	return s.private.DeleteWorkbenchDocument(ctx, runKey(value.Owner, value.Public.ID), value.Public.Revision)
}
func (s *Service) PruneRecords(ctx context.Context, owner string) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.pruneRecords(ctx, owner)
}
func (s *Service) pruneRecords(ctx context.Context, owner string) error {
	type candidate struct {
		key     string
		created time.Time
	}
	candidates := []candidate{}
	err := s.documents(ctx, "run:"+owner+":", func(record configstore.WorkbenchDocumentRecord) error {
		value, err := decodeRun(record.Payload)
		if err != nil {
			return err
		}
		s.activeMu.Lock()
		active := s.active[value.Public.ID] != nil
		s.activeMu.Unlock()
		if !active && value.Public.Action != "maintenance" {
			candidates = append(candidates, candidate{record.ID, value.Public.CreatedAt})
		}
		return nil
	})
	if err != nil {
		return err
	}
	slices.SortFunc(candidates, func(a, b candidate) int { return b.created.Compare(a.created) })
	for i := 100; i < len(candidates); i++ {
		raw, revision, err := s.private.WorkbenchDocument(ctx, candidates[i].key)
		if err != nil {
			return err
		}
		if len(raw) == 0 {
			continue
		}
		value, err := decodeRun(raw)
		if err != nil {
			return err
		}
		value.Public.Revision = revision
		if err = s.deleteRecord(ctx, &value); err != nil {
			return err
		}
	}
	return nil
}
