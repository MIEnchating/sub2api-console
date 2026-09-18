package accountworkbench

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) CheckMaintenance(ctx context.Context, owner string, revision int64) (Maintenance, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := s.checkOwner(ctx, owner); err != nil {
		return Maintenance{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return Maintenance{}, err
	}
	value, err := s.readMaintenance(ctx, target)
	if err != nil {
		return Maintenance{}, err
	}
	if value.Public.Revision == 0 || value.Public.Revision != revision {
		return Maintenance{}, configstore.ErrWorkbenchVersion
	}
	if value.Owner != owner {
		return Maintenance{}, errors.New("请先预览并保存设置，将维护委托给当前登录会话")
	}
	return s.launchMaintenance(ctx, &value)
}
func (s *Service) StopMaintenance(ctx context.Context, owner string, revision int64) (Maintenance, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := s.checkOwner(ctx, owner); err != nil {
		return Maintenance{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return Maintenance{}, err
	}
	value, err := s.readMaintenance(ctx, target)
	if err != nil {
		return Maintenance{}, err
	}
	if value.Public.Revision != revision {
		return Maintenance{}, configstore.ErrWorkbenchVersion
	}
	s.activeMu.Lock()
	active := s.active["maintenance:"+targetKey(target)]
	s.activeMu.Unlock()
	if active != nil {
		s.runner.CancelTask(active.taskID)
		select {
		case <-active.done:
		case <-ctx.Done():
			return Maintenance{}, errors.New("维护正在停止，请等待当前请求核对结束")
		}
		value, err = s.readMaintenance(ctx, target)
		if err != nil {
			return Maintenance{}, err
		}
	}
	value.Public.Enabled = false
	value.Public.NextCheckAt = nil
	value.Public.Message = "自动维护已停止"
	if err = s.saveMaintenance(ctx, &value); err != nil {
		return Maintenance{}, err
	}
	return value.Public, nil
}
func (s *Service) launchMaintenance(ctx context.Context, value *privateMaintenance) (Maintenance, error) {
	if s.runner == nil || s.tasks == nil || (value.Public.CheckAfterRepair && s.checker == nil) {
		return Maintenance{}, errors.New("维护任务服务尚未就绪")
	}
	key := "maintenance:" + targetKey(value.Target)
	s.activeMu.Lock()
	busy := s.active[key] != nil
	s.activeMu.Unlock()
	if busy {
		return value.Public, nil
	}
	if err := s.maintenanceAllowed(ctx, value); err != nil {
		return Maintenance{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: newID(), Skill: Skill, Operation: "workbench-maintenance", Status: "queued", Message: "账号维护已排队", CreatedAt: now, UpdatedAt: now, Result: map[string]any{}}
	value.Public.Running = true
	value.Public.TaskID = task.ID
	value.Public.NextCheckAt = nil
	value.Public.Results = []MaintenanceResult{}
	value.Public.Message = "账号维护已排队"
	if err := s.saveMaintenance(ctx, value); err != nil {
		return Maintenance{}, err
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		value.Public.Running = false
		value.Public.Enabled = false
		value.Public.Message = "维护任务保存失败，请重新保存设置"
		_ = s.persistMaintenance(value)
		return Maintenance{}, err
	}
	active := &activeRun{done: make(chan struct{}), owner: value.Owner, taskID: task.ID}
	s.activeMu.Lock()
	s.active[key] = active
	s.activeMu.Unlock()
	queued := value.Public
	err := s.runner.GoTask(task.ID, func(parent context.Context) {
		defer func() { s.activeMu.Lock(); delete(s.active, key); close(active.done); s.activeMu.Unlock() }()
		s.executeMaintenance(parent, value, task)
	})
	if err != nil {
		s.activeMu.Lock()
		delete(s.active, key)
		close(active.done)
		s.activeMu.Unlock()
		value.Public.Running = false
		value.Public.Enabled = false
		value.Public.Message = "维护任务未启动，请重新保存设置"
		_ = s.persistMaintenance(value)
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return Maintenance{}, errors.New("维护任务未启动")
	}
	return queued, nil
}
func (s *Service) maintenanceAllowed(ctx context.Context, value *privateMaintenance) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.checkOwner(ctx, value.Owner); err != nil {
		return err
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, value.Target), s.private); err != nil {
		return err
	}
	_, revision, err := s.private.WorkbenchDocument(ctx, "maintenance:"+targetKey(value.Target))
	if err != nil {
		return err
	}
	if revision != value.Public.Revision {
		return configstore.ErrWorkbenchVersion
	}
	return nil
}
func (s *Service) persistMaintenance(value *privateMaintenance) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.saveMaintenance(ctx, value)
}
func (s *Service) TickMaintenance(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.documents(ctx, "maintenance:", func(record configstore.WorkbenchDocumentRecord) error {
		value := defaultMaintenance()
		if decodePrivate(record.Payload, &value) != nil {
			return errors.New("维护配置无法读取")
		}
		value.Public.Revision = record.Revision
		if !value.Public.Enabled || value.Public.NextCheckAt == nil || value.Public.NextCheckAt.After(time.Now().UTC()) {
			return nil
		}
		if err := s.maintenanceAllowed(ctx, &value); err != nil {
			value.Public.Enabled = false
			value.Public.NextCheckAt = nil
			value.Public.Message = "登录会话或目标已变化，请重新预览并保存维护设置"
			return s.saveMaintenance(ctx, &value)
		}
		_, err := s.launchMaintenance(ctx, &value)
		return err
	})
}
func (s *Service) executeMaintenance(parent context.Context, value *privateMaintenance, task taskstore.Task) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	defer cancel()
	task.Status = "running"
	task.Message = "正在检查账号授权"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		value.Public.Running = false
		_ = s.persistMaintenance(value)
		return
	}
	client, err := s.client(value.Target)
	var accounts []map[string]any
	if err == nil {
		accounts, err = client.Accounts(ctx)
	}
	if err == nil && len(accounts) > 10000 {
		err = errors.New("站点账号数量超出维护读取范围")
	}
	seen := map[string]bool{}
	if err == nil {
		for _, account := range accounts {
			if !maintenanceEligible(account, value.Public.GroupIDs) {
				continue
			}
			id := text(account["id"])
			if id == "" || seen[id] || len(seen) >= 500 {
				err = errors.New("维护账号范围无效，请缩小分组范围并重新确认")
				break
			}
			seen[id] = true
			if err = s.maintenanceAllowed(ctx, value); err != nil {
				break
			}
			result := s.maintainAccount(ctx, value, account)
			value.Public.Results = append(value.Public.Results, result)
			value.States[id] = result
			if err = s.persistMaintenance(value); err != nil {
				break
			}
		}
	}
	now := time.Now().UTC()
	value.Public.Running = false
	value.Public.LastCheckAt = &now
	value.Public.Message = "本轮维护已结束"
	task.Status = "succeeded"
	for _, row := range value.Public.Results {
		if row.Status != "healthy" && row.Status != "repaired" {
			task.Status = "partial"
		}
	}
	if err != nil {
		value.Public.Message = "本轮维护已停止，请核对登录会话、站点连接与维护范围"
		task.Status = "failed"
	}
	if ctx.Err() != nil {
		value.Public.Enabled = false
		task.Status = "cancelled"
		value.Public.Message = "维护任务已停止"
	}
	if value.Public.Enabled {
		next := now.Add(time.Duration(value.Public.IntervalMinutes) * time.Minute)
		value.Public.NextCheckAt = &next
	}
	if saveErr := s.persistMaintenance(value); saveErr != nil {
		task.Status = "failed"
	}
	task.Message = value.Public.Message
	task.Result = map[string]any{"checked": len(value.Public.Results)}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistFinal(s.tasks, task)
}
