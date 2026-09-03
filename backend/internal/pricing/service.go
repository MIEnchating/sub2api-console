package pricing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type Repository interface {
	ControlPolicy(context.Context) (map[string]any, error)
	UpdatePolicy(context.Context, map[string]any, string) (business.PolicySnapshot, error)
	PricingCatalog(context.Context) (business.PricingCatalog, error)
	RevenueCatalog(context.Context) (business.RevenueCatalog, error)
	ValidateNewAPIQuotaUnit(context.Context, string, float64, time.Time, time.Time) error
	SyncPricingAccountGroups(context.Context, map[string][]string, string) (business.PricingSyncResult, error)
	CreatePricingBackup(context.Context, string, string) (business.PricingBackup, error)
	PricingBackups(context.Context) ([]business.PricingBackup, error)
	PricingBackup(context.Context, string) (business.PricingBackup, error)
	DeletePricingBackup(context.Context, string) error
}

type mutationProtectionRepository interface {
	AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error)
}

type TargetStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type AuthStore interface {
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
}

type UsageReader interface {
	ReadSub2APIKeyUsage(context.Context, configstore.AuthRecord, string, string, string) (upstreamsync.KeyUsageObservation, error)
	ReadNewAPIKeyUsage(context.Context, configstore.AuthRecord, time.Time, time.Time) (upstreamsync.NewAPIUsageObservations, error)
}

type AuthResolver interface {
	ResolveAuth(context.Context, string, string) (*configstore.AuthRecord, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type Config struct {
	Enabled               bool       `json:"enabled"`
	ProfitMargin          float64    `json:"profit_margin"`
	ExchangeGroupSets     [][]string `json:"exchange_group_sets"`
	ExchangeGroupSetNames []string   `json:"exchange_group_set_names"`
	IntervalSeconds       int        `json:"interval_seconds"`
	WriteConcurrency      int        `json:"write_concurrency"`
}

type Group struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Platform       string  `json:"platform"`
	Status         string  `json:"status"`
	RateMultiplier *string `json:"rate_multiplier"`
	Managed        bool    `json:"managed"`
	Available      bool    `json:"available"`
	Reason         *string `json:"reason"`
}

type Decision struct {
	AccountID       string   `json:"account_id"`
	AccountName     string   `json:"account_name"`
	Platform        string   `json:"platform"`
	CostMultiplier  *string  `json:"cost_multiplier"`
	CurrentGroupIDs []string `json:"current_group_ids"`
	DesiredGroupIDs []string `json:"desired_group_ids"`
	EligibleGroups  []string `json:"eligible_groups"`
	Changed         bool     `json:"changed"`
	Skipped         bool     `json:"skipped"`
	Reason          *string  `json:"reason"`
}

type Snapshot struct {
	Config      Config     `json:"config"`
	Groups      []Group    `json:"groups"`
	Decisions   []Decision `json:"decisions"`
	Accounts    int        `json:"accounts"`
	Changes     int        `json:"changes"`
	Skipped     int        `json:"skipped"`
	GeneratedAt string     `json:"generated_at"`
}

type ItemResult struct {
	AccountID string   `json:"account_id"`
	Before    []string `json:"before"`
	After     []string `json:"after"`
	Changed   bool     `json:"changed"`
	Skipped   bool     `json:"skipped,omitempty"`
	Reason    *string  `json:"reason,omitempty"`
	Error     *string  `json:"error"`
}

type Result struct {
	Requested   int                         `json:"requested"`
	Changed     int                         `json:"changed"`
	Unchanged   int                         `json:"unchanged"`
	Skipped     int                         `json:"skipped"`
	Failed      int                         `json:"failed"`
	RemoteWrite bool                        `json:"remote_write"`
	Items       []ItemResult                `json:"items"`
	LocalSync   *business.PricingSyncResult `json:"local_sync,omitempty"`
}

type Service struct {
	repository Repository
	targets    TargetStore
	tasks      TaskStore
	taskRunner taskrunner.Runner
	usage      UsageReader
	resolver   AuthResolver
	timeout    time.Duration
}

type plan struct {
	snapshot Snapshot
}

func New(repository Repository, targets TargetStore, tasks TaskStore) *Service {
	return &Service{
		repository: repository, targets: targets, tasks: tasks,
		usage: upstreamsync.NewReader(&http.Client{Timeout: 30 * time.Second}), timeout: 30 * time.Minute,
	}
}

func (s *Service) UseAuthResolver(resolver AuthResolver) { s.resolver = resolver }

func (s *Service) UseTaskRunner(runner taskrunner.Runner) { s.taskRunner = runner }

func (s *Service) CreateBackup(ctx context.Context, name, actor string) (business.PricingBackup, error) {
	return s.repository.CreatePricingBackup(ctx, name, actor)
}

func (s *Service) Backups(ctx context.Context) ([]business.PricingBackup, error) {
	return s.repository.PricingBackups(ctx)
}

func (s *Service) DeleteBackup(ctx context.Context, backupID string) error {
	return s.repository.DeletePricingBackup(ctx, strings.TrimSpace(backupID))
}

func (s *Service) RestoreBackupNow(ctx context.Context, backupID, actor string) (Result, error) {
	ctx, err := targetguard.Capture(ctx, s.targets)
	if err != nil {
		return Result{}, err
	}
	backup, err := s.repository.PricingBackup(ctx, strings.TrimSpace(backupID))
	if err != nil {
		return Result{}, err
	}
	catalog, err := s.repository.PricingCatalog(ctx)
	if err != nil {
		return Result{}, err
	}
	accounts := make(map[string]business.PricingAccount, len(catalog.Accounts))
	for _, account := range catalog.Accounts {
		accounts[account.ID] = account
	}
	groups := make(map[string]struct{}, len(catalog.Groups))
	for _, group := range catalog.Groups {
		groups[group.ID] = struct{}{}
	}
	for _, account := range catalog.Accounts {
		for _, groupID := range account.GroupIDs {
			groups[groupID] = struct{}{}
		}
	}
	decisions := make([]Decision, 0, len(backup.Accounts))
	for _, saved := range backup.Accounts {
		account, found := accounts[saved.AccountID]
		decision := Decision{AccountID: saved.AccountID, AccountName: saved.AccountName,
			CurrentGroupIDs: append([]string{}, account.GroupIDs...), DesiredGroupIDs: append([]string{}, saved.GroupIDs...)}
		switch {
		case !found:
			reason := "备份中的账号已不存在"
			decision.Skipped, decision.Reason = true, &reason
		case account.ManualPriority:
			reason := "账号处于人工优先位，备份还原不调整分组"
			decision.Skipped, decision.Reason = true, &reason
		default:
			for _, groupID := range saved.GroupIDs {
				if _, exists := groups[groupID]; !exists {
					reason := fmt.Sprintf("备份中的分组 %s 已不存在", groupID)
					decision.Skipped, decision.Reason = true, &reason
					break
				}
			}
		}
		if !decision.Skipped {
			decision.Changed = strings.Join(decision.CurrentGroupIDs, ",") != strings.Join(decision.DesiredGroupIDs, ",")
		}
		decisions = append(decisions, decision)
	}
	changes, skipped := 0, 0
	for _, decision := range decisions {
		if decision.Changed {
			changes++
		}
		if decision.Skipped {
			skipped++
		}
	}
	config := Config{WriteConcurrency: 4}
	return s.applyPlan(ctx, plan{snapshot: Snapshot{Decisions: decisions, Accounts: len(decisions), Changes: changes, Skipped: skipped}}, config, actor)
}

func (s *Service) EnqueueRestore(ctx context.Context, backupID, actor string) (taskstore.Task, error) {
	if s.tasks == nil {
		return taskstore.Task{}, errors.New("价格备份还原任务服务尚未就绪")
	}
	if _, err := s.repository.PricingBackup(ctx, strings.TrimSpace(backupID)); err != nil {
		return taskstore.Task{}, err
	}
	expectedTarget, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	id, err := randomID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: id, Skill: "sub2api-pricing", Operation: "price-group-restore", Status: "queued", Progress: 0,
		Message: "价格分组备份还原已排队", Result: map[string]any{"backup_id": backupID}, CreatedAt: now, UpdatedAt: now}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		s.executeRestore(targetguard.Expect(parent, expectedTarget), task, backupID, actor)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) executeRestore(parent context.Context, task taskstore.Task, backupID, actor string) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 20, "正在还原价格分组备份", time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	result, err := s.RestoreBackupNow(ctx, backupID, actor)
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	encoded, _ := json.Marshal(result)
	_ = json.Unmarshal(encoded, &task.Result)
	if err != nil {
		task.Status, task.Message = "failed", "价格分组备份还原失败："+err.Error()
		task.Result["error"] = err.Error()
	} else {
		task.Status = "succeeded"
		task.Message = fmt.Sprintf("价格分组备份还原完成：更新 %d，未变 %d，跳过 %d", result.Changed, result.Unchanged, result.Skipped)
	}
	taskstore.MarkCancelled(ctx, &task, "价格分组备份还原已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	config, err := ConfigFromPolicy(policy)
	if err != nil {
		return Snapshot{}, err
	}
	value, err := s.buildPlan(ctx, config)
	if err != nil {
		return Snapshot{}, err
	}
	return value.snapshot, nil
}

func (s *Service) UpdateConfig(ctx context.Context, config Config, actor string) (Snapshot, error) {
	config, err := normalizeConfig(config)
	if err != nil {
		return Snapshot{}, err
	}
	value, err := s.buildPlan(ctx, config)
	if err != nil {
		return Snapshot{}, err
	}
	if config.Enabled {
		if err := validateExchangeCatalog(config, value.snapshot.Groups); err != nil {
			return Snapshot{}, err
		}
	}
	_, err = s.repository.UpdatePolicy(ctx, map[string]any{"advanced_policy": map[string]any{
		"price_management": map[string]any{
			"enabled": config.Enabled, "profit_margin": config.ProfitMargin,
			"exchange_group_sets":      stringGroupsToAny(config.ExchangeGroupSets),
			"exchange_group_set_names": stringsToAny(config.ExchangeGroupSetNames), "interval_seconds": config.IntervalSeconds,
			"write_concurrency": config.WriteConcurrency,
		},
	}}, actor)
	if err != nil {
		return Snapshot{}, err
	}
	return value.snapshot, nil
}

func (s *Service) ApplyNow(ctx context.Context, actor string) (Result, error) {
	ctx, err := targetguard.Capture(ctx, s.targets)
	if err != nil {
		return Result{}, err
	}
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return Result{}, err
	}
	config, err := ConfigFromPolicy(policy)
	if err != nil {
		return Result{}, err
	}
	if !config.Enabled {
		return Result{}, errors.New("价格管理未开启，本次未调整账号分组")
	}
	value, err := s.buildPlan(ctx, config)
	if err != nil {
		return Result{}, err
	}
	if err := validateExchangeCatalog(config, value.snapshot.Groups); err != nil {
		return Result{}, err
	}
	return s.applyPlan(ctx, value, config, actor)
}

func (s *Service) Enqueue(ctx context.Context, actor string) (taskstore.Task, error) {
	if s.tasks == nil {
		return taskstore.Task{}, errors.New("价格管理任务服务尚未就绪")
	}
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	config, err := ConfigFromPolicy(policy)
	if err != nil {
		return taskstore.Task{}, err
	}
	if !config.Enabled {
		return taskstore.Task{}, errors.New("价格管理未开启，本次未调整账号分组")
	}
	expectedTarget, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	id, err := randomID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: id, Skill: "sub2api-pricing", Operation: "price-group-allocation", Status: "queued", Progress: 0,
		Message: "价格分组调整已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		s.execute(targetguard.Expect(parent, expectedTarget), task, actor)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) execute(parent context.Context, task taskstore.Task, actor string) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 20, "正在按盈利比例计算账号分组", time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	result, err := s.ApplyNow(ctx, actor)
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	encoded, _ := json.Marshal(result)
	_ = json.Unmarshal(encoded, &task.Result)
	if err != nil {
		task.Status, task.Message = "failed", "价格分组调整失败："+err.Error()
		task.Result["error"] = err.Error()
	} else if result.Failed > 0 {
		task.Status, task.Message = "failed", fmt.Sprintf("价格分组调整部分失败：更新 %d，失败 %d", result.Changed, result.Failed)
	} else {
		task.Status, task.Message = "succeeded", fmt.Sprintf("价格分组调整完成：更新 %d，未变 %d，跳过 %d", result.Changed, result.Unchanged, result.Skipped)
	}
	taskstore.MarkCancelled(ctx, &task, "价格分组调整已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func ConfigFromPolicy(policy map[string]any) (Config, error) {
	config := Config{ProfitMargin: 0.2, IntervalSeconds: 120, WriteConcurrency: 4, ExchangeGroupSets: [][]string{}}
	raw, present := policy["price_management"]
	if !present {
		return config, nil
	}
	section, ok := raw.(map[string]any)
	if !ok {
		return Config{}, errors.New("价格管理配置无效：price_management")
	}
	if value, found := section["enabled"]; found {
		var valid bool
		config.Enabled, valid = value.(bool)
		if !valid {
			return Config{}, errors.New("价格管理配置无效：price_management.enabled")
		}
	}
	if value, found := section["profit_margin"]; found {
		parsed, valid := number(value)
		if !valid {
			return Config{}, errors.New("价格管理配置无效：price_management.profit_margin")
		}
		config.ProfitMargin = parsed
	}
	if value, found := section["interval_seconds"]; found {
		parsed, valid := integer(value)
		if !valid {
			return Config{}, errors.New("价格管理配置无效：price_management.interval_seconds")
		}
		config.IntervalSeconds = parsed
	}
	if value, found := section["write_concurrency"]; found {
		parsed, valid := integer(value)
		if !valid {
			return Config{}, errors.New("价格管理配置无效：price_management.write_concurrency")
		}
		config.WriteConcurrency = parsed
	}
	if value, found := section["exchange_group_sets"]; found {
		items, valid := value.([]any)
		if !valid {
			return Config{}, errors.New("价格管理配置无效：price_management.exchange_group_sets")
		}
		config.ExchangeGroupSets = make([][]string, 0, len(items))
		for _, rawSet := range items {
			setItems, ok := rawSet.([]any)
			if !ok {
				return Config{}, errors.New("价格管理配置无效：price_management.exchange_group_sets")
			}
			groupSet := make([]string, 0, len(setItems))
			for _, item := range setItems {
				groupSet = append(groupSet, strings.TrimSpace(fmt.Sprint(item)))
			}
			config.ExchangeGroupSets = append(config.ExchangeGroupSets, groupSet)
		}
	} else if _, legacy := section["managed_group_ids"]; legacy {
		config.Enabled = false
	}
	if value, found := section["exchange_group_set_names"]; found {
		items, valid := value.([]any)
		if !valid {
			return Config{}, errors.New("价格管理配置无效：price_management.exchange_group_set_names")
		}
		config.ExchangeGroupSetNames = make([]string, 0, len(items))
		for _, item := range items {
			name, ok := item.(string)
			if !ok {
				return Config{}, errors.New("价格管理配置无效：price_management.exchange_group_set_names")
			}
			config.ExchangeGroupSetNames = append(config.ExchangeGroupSetNames, name)
		}
	}
	return normalizeConfig(config)
}

func normalizeConfig(config Config) (Config, error) {
	if math.IsNaN(config.ProfitMargin) || math.IsInf(config.ProfitMargin, 0) || config.ProfitMargin < 0 || config.ProfitMargin > 0.99 {
		return Config{}, errors.New("目标盈利比例必须在 0% 到 99% 之间")
	}
	if config.IntervalSeconds < 30 || config.IntervalSeconds > 86400 {
		return Config{}, errors.New("动态调整间隔必须在 30 到 86400 秒之间")
	}
	if config.WriteConcurrency < 1 || config.WriteConcurrency > 16 {
		return Config{}, errors.New("分组写入并发必须在 1 到 16 之间")
	}
	if len(config.ExchangeGroupSetNames) == 0 && len(config.ExchangeGroupSets) > 0 {
		config.ExchangeGroupSetNames = make([]string, len(config.ExchangeGroupSets))
		for index := range config.ExchangeGroupSetNames {
			config.ExchangeGroupSetNames[index] = fmt.Sprintf("互换组 %d", index+1)
		}
	}
	if len(config.ExchangeGroupSetNames) != len(config.ExchangeGroupSets) {
		return Config{}, errors.New("互换组规则名称数量必须与互换组数量一致")
	}
	type namedGroupSet struct {
		name   string
		groups []string
	}
	seen := map[string]struct{}{}
	sets := make([]namedGroupSet, 0, len(config.ExchangeGroupSets))
	for setIndex, values := range config.ExchangeGroupSets {
		name := strings.TrimSpace(config.ExchangeGroupSetNames[setIndex])
		if name == "" || len([]rune(name)) > 64 {
			return Config{}, fmt.Errorf("互换组 %d 的规则名称不能为空且不能超过 64 个字符", setIndex+1)
		}
		groupSet := make([]string, 0, len(values))
		localSeen := map[string]struct{}{}
		for _, value := range values {
			value = strings.TrimSpace(value)
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
				return Config{}, errors.New("互换组分组必须使用稳定正整数 ID")
			}
			if _, duplicate := localSeen[value]; duplicate {
				continue
			}
			if _, duplicate := seen[value]; duplicate {
				return Config{}, fmt.Errorf("分组 %s 不能同时属于多个互换组", value)
			}
			localSeen[value] = struct{}{}
			seen[value] = struct{}{}
			groupSet = append(groupSet, value)
		}
		if len(groupSet) < 2 {
			return Config{}, errors.New("每个互换组至少需要两个分组")
		}
		sort.Slice(groupSet, func(left, right int) bool { return numericLess(groupSet[left], groupSet[right]) })
		sets = append(sets, namedGroupSet{name: name, groups: groupSet})
	}
	sort.Slice(sets, func(left, right int) bool { return numericLess(sets[left].groups[0], sets[right].groups[0]) })
	config.ExchangeGroupSets = make([][]string, 0, len(sets))
	config.ExchangeGroupSetNames = make([]string, 0, len(sets))
	for _, set := range sets {
		config.ExchangeGroupSets = append(config.ExchangeGroupSets, set.groups)
		config.ExchangeGroupSetNames = append(config.ExchangeGroupSetNames, set.name)
	}
	if config.Enabled && len(sets) == 0 {
		return Config{}, errors.New("开启价格管理前至少配置一个账号互换组")
	}
	return config, nil
}

func (s *Service) buildPlan(ctx context.Context, config Config) (plan, error) {
	catalog, err := s.repository.PricingCatalog(ctx)
	if err != nil {
		return plan{}, fmt.Errorf("Console 本地价格目录读取失败：%w", err)
	}
	snapshot, err := evaluate(config, catalog)
	if err != nil {
		return plan{}, err
	}
	return plan{snapshot: snapshot}, nil
}

func evaluate(config Config, catalog business.PricingCatalog) (Snapshot, error) {
	managed := map[string]struct{}{}
	setByGroup := map[string]int{}
	for setIndex, groupSet := range config.ExchangeGroupSets {
		for _, groupID := range groupSet {
			managed[groupID] = struct{}{}
			setByGroup[groupID] = setIndex
		}
	}
	groupByID := map[string]Group{}
	groupPrice := map[string]*big.Rat{}
	resultGroups := make([]Group, 0, len(catalog.Groups))
	for _, raw := range catalog.Groups {
		id := strings.TrimSpace(raw.ID)
		if !stableID(id) {
			continue
		}
		item := Group{ID: id, Name: strings.TrimSpace(raw.Name), Platform: strings.ToLower(strings.TrimSpace(raw.Platform)), Status: "active"}
		_, item.Managed = managed[id]
		rateText := ""
		if raw.RateMultiplier != nil {
			rateText = strings.TrimSpace(*raw.RateMultiplier)
		}
		if rate, ok := positiveRat(rateText); ok {
			item.RateMultiplier = &rateText
			groupPrice[id] = rate
		}
		item.Available = item.Status == "active" && item.Platform != "" && item.Platform != "composite" && groupPrice[id] != nil
		if !item.Available {
			reason := "分组不可用于自动价格分配"
			switch {
			case item.Platform == "composite":
				reason = "复合分组由模型路由控制，不自动调整账号成员"
			case item.Status != "active":
				reason = "分组未启用"
			case groupPrice[id] == nil:
				reason = "分组售价倍率无效"
			}
			item.Reason = &reason
		}
		groupByID[id] = item
		resultGroups = append(resultGroups, item)
	}
	for id := range managed {
		if _, found := groupByID[id]; !found {
			reason := "分组已不存在，请从受管分组中移除"
			item := Group{ID: id, Managed: true, Available: false, Reason: &reason}
			groupByID[id] = item
			resultGroups = append(resultGroups, item)
		}
	}
	sort.Slice(resultGroups, func(i, j int) bool { return numericLess(resultGroups[i].ID, resultGroups[j].ID) })
	margin := new(big.Rat)
	margin.SetString(strconv.FormatFloat(config.ProfitMargin, 'f', -1, 64))
	onePlusMargin := new(big.Rat).Add(big.NewRat(1, 1), margin)
	decisions := make([]Decision, 0, len(catalog.Accounts))
	changes, skipped := 0, 0
	for _, account := range catalog.Accounts {
		id := strings.TrimSpace(account.ID)
		if !stableID(id) {
			continue
		}
		current := append([]string{}, account.GroupIDs...)
		decision := Decision{AccountID: id, AccountName: strings.TrimSpace(account.Name), Platform: strings.ToLower(strings.TrimSpace(account.Platform)), CurrentGroupIDs: current, DesiredGroupIDs: append([]string{}, current...), EligibleGroups: []string{}}
		costText := ""
		if account.Multiplier != nil {
			costText = strings.TrimSpace(*account.Multiplier)
		}
		cost, validCost := positiveRat(costText)
		if strings.TrimSpace(costText) != "" {
			decision.CostMultiplier = &costText
		}
		if account.ManualPriority {
			reason := "账号处于人工优先位，价格管理不调整分组"
			decision.Skipped, decision.Reason = true, &reason
			skipped++
			decisions = append(decisions, decision)
			continue
		}
		if !account.GroupsValid || !validCost || decision.Platform == "" {
			reason := "账号价格信息不完整，保留原分组"
			switch {
			case !account.GroupsValid:
				reason = "账号当前分组数据无效，保留原分组"
			case strings.TrimSpace(costText) == "":
				reason = "账号未设置成本倍率，保留原分组"
			case !validCost:
				reason = fmt.Sprintf("账号成本倍率 %s 无效，必须大于 0，保留原分组", costText)
			case decision.Platform == "":
				reason = "账号平台缺失，保留原分组"
			}
			decision.Skipped, decision.Reason = true, &reason
			skipped++
			decisions = append(decisions, decision)
			continue
		}
		desired := make([]string, 0, len(current)+len(managed))
		activeSets := map[int]struct{}{}
		currentBySet := map[int][]string{}
		for _, groupID := range current {
			setIndex, controlled := setByGroup[groupID]
			if controlled {
				activeSets[setIndex] = struct{}{}
				currentBySet[setIndex] = append(currentBySet[setIndex], groupID)
			}
			if !controlled {
				desired = append(desired, groupID)
			}
		}
		activeSetIndexes := make([]int, 0, len(activeSets))
		for setIndex := range activeSets {
			activeSetIndexes = append(activeSetIndexes, setIndex)
		}
		sort.Ints(activeSetIndexes)
		for _, setIndex := range activeSetIndexes {
			chosenID := ""
			var chosenRate *big.Rat
			compatible := 0
			for _, groupID := range config.ExchangeGroupSets[setIndex] {
				group := groupByID[groupID]
				if !group.Available || group.Platform != decision.Platform {
					continue
				}
				compatible++
				limit := new(big.Rat).Quo(groupPrice[groupID], onePlusMargin)
				if cost.Cmp(limit) > 0 {
					continue
				}
				rate := groupPrice[groupID]
				if chosenRate == nil || rate.Cmp(chosenRate) < 0 || (rate.Cmp(chosenRate) == 0 && numericLess(groupID, chosenID)) {
					chosenID, chosenRate = groupID, rate
				}
			}
			profitableChosen := chosenID != ""
			if chosenID == "" && len(currentBySet[setIndex]) > 0 {
				preserved := append([]string{}, currentBySet[setIndex]...)
				sort.Slice(preserved, func(left, right int) bool { return numericLess(preserved[left], preserved[right]) })
				chosenID = preserved[0]
				if compatible > 0 {
					reason := "没有满足盈利比例的可用分组，保留当前分组"
					decision.Reason = &reason
				}
			}
			if chosenID != "" {
				desired = append(desired, chosenID)
				if profitableChosen {
					decision.EligibleGroups = append(decision.EligibleGroups, groupByID[chosenID].Name)
				}
			}
		}
		sort.Slice(desired, func(i, j int) bool { return numericLess(desired[i], desired[j]) })
		decision.DesiredGroupIDs = desired
		decision.Changed = strings.Join(current, ",") != strings.Join(desired, ",")
		if decision.Changed {
			changes++
		}
		decisions = append(decisions, decision)
	}
	sort.Slice(decisions, func(i, j int) bool { return numericLess(decisions[i].AccountID, decisions[j].AccountID) })
	return Snapshot{Config: config, Groups: resultGroups, Decisions: decisions, Accounts: len(decisions), Changes: changes, Skipped: skipped, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

func validateExchangeCatalog(config Config, groups []Group) error {
	byID := make(map[string]Group, len(groups))
	for _, group := range groups {
		byID[group.ID] = group
	}
	for setIndex, groupSet := range config.ExchangeGroupSets {
		platform := ""
		for _, groupID := range groupSet {
			group, found := byID[groupID]
			if !found {
				return fmt.Errorf("互换组 %d 中的分组 %s 不在 Console 本地目录中", setIndex+1, groupID)
			}
			if !group.Available {
				return fmt.Errorf("互换组 %d 中的分组 %s 当前不可分配：%s", setIndex+1, group.Name, pointerText(group.Reason))
			}
			if platform == "" {
				platform = group.Platform
			} else if platform != group.Platform {
				return fmt.Errorf("互换组 %d 不能混合不同平台的分组", setIndex+1)
			}
		}
	}
	return nil
}

func pointerText(value *string) string {
	if value == nil {
		return "原因未记录"
	}
	return *value
}

func (s *Service) applyPlan(ctx context.Context, value plan, config Config, actor string) (Result, error) {
	if err := validatePriceGroupUniqueness(value.snapshot.Decisions, config); err != nil {
		return Result{}, err
	}
	guarded, release, err := s.acquirePlanMutation(ctx, value.snapshot.Decisions)
	if err != nil {
		return Result{}, err
	}
	defer release()
	ctx = guarded
	ctx, err = targetguard.Bind(ctx, s.targets)
	if err != nil {
		return Result{}, err
	}
	settings, err := targetguard.Settings(ctx, s.targets)
	if err != nil {
		return Result{}, err
	}
	client, err := adminclient.New(adminclient.Config{BaseURL: settings.BaseURL, AdminKey: settings.AdminKey, Timeout: time.Duration(settings.TimeoutSeconds) * time.Second, Attempts: 1}, nil)
	if err != nil {
		return Result{}, err
	}
	result := Result{Requested: value.snapshot.Accounts, Skipped: value.snapshot.Skipped, Items: []ItemResult{}}
	jobs := make(chan Decision)
	items := make(chan ItemResult, value.snapshot.Changes)
	var workers sync.WaitGroup
	for index := 0; index < config.WriteConcurrency; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for decision := range jobs {
				reason, protectionErr := s.pricingMutationProtection(ctx, decision.AccountID)
				if protectionErr != nil {
					message := protectionErr.Error()
					items <- ItemResult{AccountID: decision.AccountID, Before: decision.CurrentGroupIDs, After: decision.DesiredGroupIDs, Error: &message}
					continue
				}
				if reason != nil {
					items <- ItemResult{AccountID: decision.AccountID, Before: decision.CurrentGroupIDs, After: decision.CurrentGroupIDs, Skipped: true, Reason: reason}
					continue
				}
				currentGroupIDs, baselineErr := pricingAccountGroupIDs(ctx, client, decision.AccountID)
				if baselineErr != nil {
					message := baselineErr.Error()
					items <- ItemResult{AccountID: decision.AccountID, Before: decision.CurrentGroupIDs, After: decision.DesiredGroupIDs, Error: &message}
					continue
				}
				if !sameGroupIDs(currentGroupIDs, decision.CurrentGroupIDs) {
					reason := "账号分组在价格计划生成后已变化，价格管理未调整分组"
					items <- ItemResult{AccountID: decision.AccountID, Before: currentGroupIDs, After: currentGroupIDs, Skipped: true, Reason: &reason}
					continue
				}
				ids := make([]int64, len(decision.DesiredGroupIDs))
				for index, value := range decision.DesiredGroupIDs {
					ids[index], _ = strconv.ParseInt(value, 10, 64)
				}
				_, writeErr := client.UpdateAccountGroups(ctx, decision.AccountID, ids)
				item := ItemResult{AccountID: decision.AccountID, Before: decision.CurrentGroupIDs, After: decision.DesiredGroupIDs, Changed: writeErr == nil}
				if writeErr != nil {
					message := writeErr.Error()
					item.Error = &message
				}
				items <- item
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, decision := range value.snapshot.Decisions {
			if decision.Changed && !decision.Skipped {
				jobs <- decision
			}
		}
	}()
	go func() { workers.Wait(); close(items) }()
	syncedGroups := map[string][]string{}
	for item := range items {
		result.Items = append(result.Items, item)
		if item.Skipped {
			result.Skipped++
		} else if item.Error != nil {
			result.Failed++
		} else {
			result.Changed++
			result.RemoteWrite = true
			syncedGroups[item.AccountID] = append([]string{}, item.After...)
		}
	}
	result.Unchanged = result.Requested - result.Skipped - result.Changed - result.Failed
	if result.Unchanged < 0 {
		result.Unchanged = 0
	}
	sort.Slice(result.Items, func(i, j int) bool { return numericLess(result.Items[i].AccountID, result.Items[j].AccountID) })
	if result.Changed > 0 {
		local, syncErr := s.repository.SyncPricingAccountGroups(ctx, syncedGroups, actor)
		release()
		if syncErr != nil {
			return result, fmt.Errorf("远程分组已更新，但本地目录同步失败：%w", syncErr)
		}
		result.LocalSync = &local
	} else {
		release()
	}
	if result.Failed > 0 {
		return result, fmt.Errorf("%d 个账号分组调整失败", result.Failed)
	}
	return result, nil
}

func (s *Service) acquirePlanMutation(ctx context.Context, decisions []Decision) (context.Context, func(), error) {
	resources := make([]string, 0, len(decisions))
	seen := make(map[string]struct{}, len(decisions))
	for _, decision := range decisions {
		if !decision.Changed || decision.Skipped {
			continue
		}
		resource := mutationguard.Account(decision.AccountID)
		if _, duplicate := seen[resource]; duplicate {
			continue
		}
		seen[resource] = struct{}{}
		resources = append(resources, resource)
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return nil, nil, err
	}
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			if err := release(); err != nil {
				slog.Error("价格管理账号租约释放失败", "resources", resources, "error", err)
			}
		})
	}
	return guarded, cleanup, nil
}

func (s *Service) pricingMutationProtection(ctx context.Context, accountID string) (*string, error) {
	if repository, ok := s.repository.(mutationProtectionRepository); ok {
		protection, err := repository.AccountMutationProtection(ctx, accountID)
		if err != nil {
			return nil, fmt.Errorf("人工保护状态复核失败：%w", err)
		}
		if protection.Protected() {
			reason := "账号已启用" + strings.Join(protection.Reasons(), "、") + "，价格管理未调整分组"
			return &reason, nil
		}
	}
	return nil, nil
}

func pricingAccountGroupIDs(ctx context.Context, client *adminclient.Client, accountID string) ([]string, error) {
	account, err := client.Account(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("账号分组基线复核失败：%w", err)
	}
	values, ok := account["group_ids"].([]any)
	if !ok {
		return nil, errors.New("账号分组基线复核失败：远程账号分组格式不可读")
	}
	groupIDs := make([]string, 0, len(values))
	for _, value := range values {
		groupID := strings.TrimSpace(fmt.Sprint(value))
		if !stableID(groupID) {
			return nil, errors.New("账号分组基线复核失败：远程账号分组包含无效稳定 ID")
		}
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(left, right int) bool { return numericLess(groupIDs[left], groupIDs[right]) })
	return groupIDs, nil
}

func sameGroupIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string{}, left...)
	right = append([]string{}, right...)
	sort.Slice(left, func(i, j int) bool { return numericLess(left[i], left[j]) })
	sort.Slice(right, func(i, j int) bool { return numericLess(right[i], right[j]) })
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validatePriceGroupUniqueness(decisions []Decision, config Config) error {
	setByGroup := make(map[string]int)
	for setIndex, groupSet := range config.ExchangeGroupSets {
		for _, groupID := range groupSet {
			setByGroup[groupID] = setIndex
		}
	}
	for _, decision := range decisions {
		if decision.Skipped || !decision.Changed {
			continue
		}
		if len(decision.DesiredGroupIDs) == 0 {
			return fmt.Errorf("账号 %s 的目标分组为空，已拒绝写入", decision.AccountID)
		}
		counts := make(map[int]int)
		for _, groupID := range decision.DesiredGroupIDs {
			setIndex, controlled := setByGroup[groupID]
			if !controlled {
				continue
			}
			counts[setIndex]++
			if counts[setIndex] > 1 {
				return fmt.Errorf("账号 %s 在互换组 %d 中生成了多个目标分组，已拒绝写入", decision.AccountID, setIndex+1)
			}
		}
	}
	return nil
}

func number(value any) (float64, bool) {
	switch item := value.(type) {
	case float64:
		return item, !math.IsNaN(item) && !math.IsInf(item, 0)
	case int:
		return float64(item), true
	case int64:
		return float64(item), true
	case json.Number:
		parsed, err := item.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func integer(value any) (int, bool) {
	parsed, valid := number(value)
	return int(parsed), valid && parsed == math.Trunc(parsed)
}

func positiveRat(value string) (*big.Rat, bool) {
	parsed := new(big.Rat)
	if _, ok := parsed.SetString(strings.TrimSpace(value)); !ok || parsed.Sign() <= 0 {
		return nil, false
	}
	return parsed, true
}

func stableID(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func numericLess(left, right string) bool {
	leftValue, leftErr := strconv.ParseInt(left, 10, 64)
	rightValue, rightErr := strconv.ParseInt(right, 10, 64)
	if leftErr == nil && rightErr == nil {
		return leftValue < rightValue
	}
	return left < right
}

func stringGroupsToAny(groups [][]string) []any {
	result := make([]any, len(groups))
	for index, values := range groups {
		items := make([]any, len(values))
		for itemIndex, value := range values {
			items[itemIndex] = value
		}
		result[index] = items
	}
	return result
}

func stringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func randomID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
