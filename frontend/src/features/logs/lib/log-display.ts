import type { UnifiedLogEntry, UnifiedLogEventLevel, UnifiedLogKind, UnifiedLogState } from "@/api";
import type { StatusVariant } from "@/components/status-badge";

const kindLabels: Record<UnifiedLogKind, string> = {
  all: "全部记录",
  task: "任务记录",
  event: "事件日志",
  change: "远程读写",
};

const titleLabels: Record<string, string> = {
  "latency-watcher": "延迟监控",
  "routing-decision-change": "路由决策变更",
  "upstream-runtime-snapshot": "上游运行快照",
  "upstream-sync-run": "上游同步任务",
  "inspection-coordinator-run": "巡检协调任务",
  "auth-recovery-run": "鉴权恢复任务",
  "auth-recovery-runtime-snapshot": "鉴权恢复快照",
  "active-probe": "主动探测",
  "account-delete-batch": "批量删除账号及上游 Key",
  "account-model-behavior-check": "账号模型检测",
  "discover-notification-target": "获取 QQBot 通知目标",
  evaluate: "告警检测",
  "price-group-allocation": "分配价格分组",
  "price-group-restore": "恢复价格分组",
  "recover-hosts": "批量鉴权恢复",
  "revenue-calculation": "收入核算",
  "automatic-inspection": "自动巡检",
  "manual-inspection": "手动巡检",
  "recover-host": "鉴权恢复",
  "management-snapshot-sync": "账号与分组同步",
  "account-rate-sync": "账号倍率同步",
  "upstream-balances-sync": "上游余额同步",
  "upstream-groups-sync": "上游分组同步",
  "balance-sync": "余额同步",
  "upstream-sync": "上游同步",
  "upstream-delete": "删除上游",
  "account-delete": "删除账号及上游 Key",
  "alerts.evaluate": "告警检测",
  "auth-recovery": "鉴权恢复",
  "inspection-run": "巡检任务",
  "inspection.snapshot-review": "巡检快照复核",
  "runtime.inspection": "巡检运行",
  inspection: "自动巡检",
  alerts: "告警检测",
  auth_recovery: "鉴权恢复",
  account_sync: "账号同步",
  rate_sync: "倍率同步",
  "policy.updated": "策略更新",
  "upstream.sync": "上游同步",
  "upstream.created": "添加上游",
  "upstream.deleted": "删除上游",
  "upstream.configuration.updated": "更新上游配置",
  "management.snapshot.synced": "账号与分组同步",
  "account.rates.synced": "账号倍率同步",
  "group.scope.updated": "更新守护范围",
  "inspection.automation.resumed": "恢复自动巡检",
  "inspection.automation.cancelled": "取消自动巡检",
  "inspection.automation.updated": "更新自动巡检配置",
  "rate-sync": "倍率同步",
  "routing.writeback": "自动执行",
  远程读取复核: "远程读取复核",
  远程写入: "远程写入",
  写后复核: "写后复核",
  远程写入与复核: "远程写入与复核",
  "routing.writeback.batch": "批量自动执行",
  "account.scheduling": "账号调度",
  "account-scheduling": "账号调度",
  "account.sync": "账号同步",
  "account-sync": "账号字段同步",
  "account.groups.sync": "账号分组同步",
  "account.onboarding": "账号添加",
  "upstream.delete": "删除上游",
  "upstream.rate_sync": "上游倍率同步",
  "notifications.test": "通知测试",
  "account.control": "账号调度控制",
  "account.test_model": "更新探测模型",
  "group.policy.updated": "更新分组策略",
  "group.policy.cleared": "恢复全局分组策略",
  "routing.degraded": "账号降级",
  "routing.fused": "停止调度",
  "routing.survivor": "保底强留",
  "routing.recovered": "恢复调度",
  "routing.applied": "自动写回成功",
  "routing.apply_failed": "自动写回失败",
  "probe.failed": "主动探测失败",
  probe_failed: "主动探测失败",
  probe_model_rewritten: "探测模型已调整",
  cleanup_queued: "进入自动处置队列",
  cleanup_skipped: "跳过自动处置",
  cleanup_deferred: "延后自动处置",
  cleanup_paused: "自动暂停账号",
  cleanup_disabled: "自动停用账号",
  cleanup_deleted: "自动删除账号",
  cleanup_failed: "自动处置失败",
  cleanup_delete_pending: "准备自动删除账号",
  cleanup_delete_failed: "自动删除账号失败",
  cleanup_predisable_failed: "删除前停止调度失败",
  routing_recovery_cleanup_failed: "恢复调度后清理状态失败",
  upstream_sync: "上游数据同步",
  upstream_rate_sync: "上游数据同步",
  account_rate_sync: "账号倍率与名称同步",
  price_management: "价格分组调整",
  traffic_refresh: "真实流量同步",
  active_probe: "主动探测",
  evidence_collection: "请求记录与探针",
  routing_calculation: "调度计算",
  inspection_calculation: "调度计算",
  routing_writeback: "自动执行",
  alert_evaluation: "告警检测",
  disk_evaluation: "磁盘检查",
};

const operationTypeLabels: Record<string, string> = {
  "routing.writeback": "自动调度写回",
  "routing.restore": "交还调度控制",
  "routing.writeback.batch": "批量自动调度",
  "account.scheduling": "账号调度",
  "account.sync": "账号字段同步",
  "account.rate.sync": "账号倍率同步",
  "account.groups.sync": "账号分组同步",
  "account.onboarding": "添加账号",
  "account.delete": "手动删除账号",
  "cleanup.delete": "自动删除账号",
  "upstream.delete": "删除上游",
  "upstream.rate_sync": "上游倍率同步",
  "notifications.test": "通知测试",
};

const phaseLabels: Record<string, string> = {
  credentials: "准备凭据",
  testing: "执行检测",
  calculation: "本地计算",
  writeback: "远程写入",
  "remote-write": "远程写入",
  readback: "读取复核",
  "remote-readback": "写后复核",
  "local-commit": "保存本地结果",
  "local-cleanup": "清理本地状态",
  evaluate: "告警检测",
  queued: "等待执行",
  running: "执行中",
  completed: "执行完成",
  quick: "快速检测",
  full: "完整检测",
};

const skillLabels: Record<string, string> = {
  console: "控制台",
  qqbot: "QQBot 通知",
  "sub2api-account-management": "账号管理",
  "sub2api-auto-inspection": "自动巡检",
  "sub2api-model-check": "模型检测",
  "sub2api-upstream-auth": "上游鉴权",
  "sub2api-upstream-info": "上游信息同步",
  "sub2api-connectivity-test": "连通性探测",
  "sub2api-operations": "账号与分组管理",
};

const sourceValueLabels: Record<string, string> = {
  console: "控制台",
  "console-domain-db": "Console 业务数据库",
  "official-account-test": "官方账号测试",
  upstream_live: "上游实时数据",
  explicit: "显式配置",
  fixed: "固定配置",
  "newapi-token-flow": "New API Token 流量",
  "sub2api-key-stats": "Sub2API 密钥统计",
  "system-log": "系统日志",
  "traffic+active_probe": "真实流量 + 主动探测",
  active_probe: "主动探测",
  task: "业务任务",
  run_record: "运行摘要",
  runtime_event: "事件日志",
  operation_audit: "远程操作审计",
  traffic: "真实流量",
  probe: "主动探测",
  "active-probe": "主动探测",
  logs: "运行日志",
  manual: "手动操作",
};

const commonValueLabels: Record<string, string> = {
  active: "启用",
  inactive: "停用",
  enabled: "启用",
  disabled: "停用",
  healthy: "健康",
  degraded: "降级",
  fused: "已熔断",
  cost_blocked: "成本墙拦截",
  observing: "观察中",
  paused: "已暂停",
  excluded: "已排除",
  deleted: "已删除",
  live: "实时探测",
  last_successful: "最近成功值",
  group_catalog: "分组目录",
  pause: "暂停调度",
  resume: "恢复调度",
  exclude: "移出守护范围",
  include: "加入守护范围",
  fuse: "停止调度",
  recover: "恢复调度",
  pass: "通过",
  consistent: "一致",
  inconsistent: "不一致",
  inconclusive: "无法判定",
  survivor: "保底强留",
  binding_invalid: "绑定无效",
  unknown: "未标记",
};

const contextualValueLabels: Record<string, Record<string, string>> = {
  attribution_level: {
    key: "按上游 Key 精确归因",
    unavailable: "无法精确归因",
  },
  auth_method: {
    newapi_admin_key: "New API 管理密钥",
    newapi_manual_login: "New API 手动登录",
    newapi_user_login: "New API 用户登录",
    newapi_user_token: "New API 用户令牌",
    sub2api_manual_login: "Sub2API 手动登录",
    sub2api_user_login: "Sub2API 用户登录",
    sub2api_user_token: "Sub2API 用户令牌",
  },
  checker: {
    claude: "Claude 检测器",
    sol: "Sol 检测器",
  },
  code: {
    browser_challenge_required: "需要浏览器验证",
    credential_commit_failed: "凭据保存失败",
    image_captcha_prepare_failed: "图片验证码准备失败",
    image_captcha_required: "需要图片验证码",
    login_agreement_required: "需要同意登录协议",
    recovered_by_refresh: "刷新令牌恢复成功",
    recovered_by_vault: "密码箱登录恢复成功",
    recovery_preference_commit_failed: "恢复方式保存失败",
    vault_entry_required: "需要选择密码箱项",
    vault_entry_unavailable: "密码箱项不可用",
    vault_login_failed: "密码箱登录失败",
  },
  cost_tier: {
    above: "高于成本墙",
    below: "低于成本墙",
    equal: "等于成本墙",
    unknown: "无法判定",
  },
  interaction_kind: {
    browser_challenge_required: "需要浏览器验证",
    image_captcha_ocr: "图片验证码识别",
    image_captcha_required: "需要图片验证码",
  },
  latest_event: {
    fatal: "致命异常",
    gateway_error: "网关错误",
    perfect: "运行正常",
    probe_fail: "主动探测失败",
    quota_exhausted: "额度耗尽",
    slow_ttfb: "首字延迟过高",
    upstream_unknown: "上游状态未知",
  },
  effective_source: {
    active_probe: "主动探测",
    traffic: "真实流量",
    "traffic+active_probe": "真实流量 + 主动探测",
  },
  observation_source: {
    group_catalog: "分组目录倍率",
    last_successful: "最近一次成功倍率",
    live: "实时探测倍率",
  },
  requested_source: {
    active_probe: "主动探测",
    traffic: "真实流量",
  },
  protocol: {
    "anthropic-messages": "Anthropic Messages 协议",
    "openai-responses": "OpenAI Responses 协议",
  },
  refresh_kind: {
    refresh: "刷新令牌",
    refresh_token: "刷新令牌",
    vault: "密码箱登录",
  },
  scope: {
    "behavioral match against a private standard; not model identity proof":
      "与内部标准进行行为特征对比，不代表模型身份认证",
    "closed-set behavioral similarity; not model identity proof":
      "封闭候选集行为相似度，不代表模型身份认证",
  },
  strategy: {
    balanced: "价格与速度均衡",
    price_first: "价格优先",
    speed_first: "速度优先",
  },
  target_type: {
    c2c: "私聊",
    channel: "频道",
    group: "群聊",
  },
  verdict: {
    error: "检测失败",
    group_match: "与同系列模型特征一致",
    inconclusive: "证据不足，无法判定",
    luna_like: "更接近 Luna",
    match: "与声明模型特征一致",
    sol_consistent: "与 Sol 特征一致",
    terra_like: "更接近 Terra",
  },
};

const statusLabels: Record<string, string> = {
  queued: "等待执行",
  running: "执行中",
  waiting_input: "等待输入",
  pending: "等待处理",
  succeeded: "成功",
  success: "成功",
  ok: "正常",
  failed: "失败",
  error: "失败",
  cancelled: "已取消",
  warning: "警告",
  partial: "部分完成",
  degraded: "已降级",
  unknown: "未标记",
};

const stateLabels: Record<UnifiedLogState, string> = {
  all: "全部",
  active: "进行中",
  failed: "失败",
  warning: "警告",
  succeeded: "成功",
};

const eventLevelLabels: Record<UnifiedLogEventLevel, string> = {
  all: "全部级别",
  info: "信息",
  warning: "警告",
  error: "错误",
};

const sourceLabels: Record<UnifiedLogEntry["source"], string> = {
  task: "业务任务",
  run_record: "运行摘要",
  runtime_event: "事件日志",
  operation_audit: "远程操作审计",
};

const detailLabels: Record<string, string> = {
  skill: "业务能力",
  operation: "执行操作",
  progress: "执行进度",
  created_at: "创建时间",
  task_name: "任务类型",
  stage: "执行阶段",
  started_at: "开始时间",
  ended_at: "结束时间",
  duration_seconds: "耗时",
  event_type: "事件类型",
  operation_id: "操作标识",
  operation_type: "操作类型",
  phase: "执行阶段",
  request_id: "请求标识",
  source: "数据来源",
  remote_confirmed: "远程确认",
  readback_confirmed: "读回确认",
  result: "任务结果",
  payload: "详细结果",
  host: "上游",
  account_id: "账号 ID",
  account_name: "账号名称",
  priority: "优先级",
  load_factor: "负载因子",
  concurrency: "并发上限",
  schedulable: "调度状态",
  findings: "发现告警",
  mode: "运行模式",
  run_key: "运行编号",
  groups: "分组",
  group_names: "分组",
  group_name: "分组",
  account_count: "账号数量",
  changed: "变更数量",
  restored: "恢复数量",
  succeeded: "成功数量",
  failed: "失败数量",
  remote_write: "远程写入",
  calculation_only: "仅本地计算",
  desired: "目标值",
  effective: "生效值",
  reason: "原因",
  error: "错误原因",
  status: "状态",
  state: "状态",
  action: "操作",
  multiplier: "账号倍率",
  rate_multiplier: "账号倍率",
  notes: "备注",
  deleted: "删除状态",
  actor: "执行人",
  key_id: "密钥 ID",
  key_count: "密钥数量",
  group_count: "分组数量",
  catalog: "分组目录",
  balance: "余额",
  auth_recovered: "鉴权已恢复",
  authentication_failure: "鉴权失败",
  evaluation_disabled: "告警检测已停用",
  delivery: "通知投递",
  strategy: "策略",
  active_run_cancelled: "已取消运行任务",
  private_auth_deleted: "已删除私有鉴权",
  interval_seconds: "执行间隔",
  group_links: "分组关联",
  group_id: "分组 ID",
  excluded: "已排除",
  enabled: "已启用",
  deleted_groups: "已删除分组数",
  deleted_accounts: "已删除账号数",
  accounts: "账号",
  operations: "执行项目",
  operation_timings: "各项目耗时",
  evidence: "采集证据",
  upstream_sync: "上游同步",
  routing: "调度计算",
  alert_evaluation: "告警检测",
  planned_operations: "计划执行项目",
  completed_operations: "已完成项目",
  active_operations: "正在执行项目",
  writeback: "远程写回",
  credentials_persisted: "凭据已保存",
  total: "总数",
  hosts: "上游",
  credentials_exposed: "凭据已暴露",
  auth_failed: "鉴权失败数",
  cancelled: "已取消数量",
  projection: "本地数据更新",
  outcome: "执行结果",
  balance_sync: "余额同步",
  interrupted: "已中断数量",
  targets: "目标对象",
  skipped: "已跳过数量",
  policy: "执行策略",
  persisted: "已保存数量",
  passed: "通过数量",
  event_id: "事件 ID",
  account_total: "账号总数",
  account_rate_succeeded: "倍率同步成功数",
  account_rate_failed: "倍率同步失败数",
  remote_deleted_accounts: "远程删除账号数",
  read_only: "只读模式",
  captcha_recovery: "验证码恢复",
  account_ids: "账号 ID 列表",
  account_rate_sync_error: "账号倍率同步错误",
  account_rate_sync_task_id: "账号倍率同步任务 ID",
  attempted: "尝试发送",
  attempts: "尝试次数",
  audit_failed: "审计失败数量",
  base_url_failed: "Base URL 读取失败",
  base_url_resolved: "Base URL 已读取",
  base_url_unavailable: "Base URL 未返回",
  batches: "投递批次",
  bound: "已绑定数量",
  captured_at: "获取时间",
  cleaned: "已清理数量",
  cleanup_warnings: "清理警告",
  combinations: "检测组合数量",
  completed: "已完成数量",
  configured: "通知已配置",
  coverage_percent: "有效响应覆盖率",
  disabled: "已停用",
  dry_run: "试运行",
  exact_model_resolved: "已精确识别模型",
  failure_reason: "失败原因",
  identity_group: "模型特征组",
  identity_match_percent: "身份特征匹配率",
  latency_p50_ms: "P50 延迟",
  latency_p95_ms: "P95 延迟",
  latency_p99_ms: "P99 延迟",
  local_group_ids: "本地分组 ID",
  local_group_names: "本地分组",
  management_account_delete_requested: "管理账号已请求删除",
  management_account_deleted: "管理账号已删除",
  management_account_readback_failed: "管理账号写后复核失败",
  management_account_still_readable: "管理账号仍可读取",
  management_delete_response_confirmed: "管理账号删除响应已确认",
  message_id: "消息 ID",
  message_ids: "消息 ID",
  model_count: "模型数量",
  model_rewritten: "响应模型已改写",
  nearest_outside_model: "最接近的组外模型",
  observed_at: "探测时间",
  outcomes: "执行结果明细",
  parameters_error: "默认参数修复错误",
  parameters_failed: "默认参数修复失败",
  parameters_repaired: "默认参数已修复",
  parameters_skipped: "默认参数已跳过",
  parameters_unchanged: "默认参数无需修改",
  renamed: "已重命名数量",
  requested_rounds: "计划检测轮数",
  resolved: "已读取数量",
  response_models: "响应模型",
  saved: "已保存",
  same_standard_percent: "同标准模型匹配率",
  sent: "发送成功",
  standard_model: "标准模型",
  suppressed: "已抑制",
  sync_balance_multiplier: "同步余额倍率",
  target_id: "目标 ID",
  target_type: "目标类型",
  tests: "检测明细",
  upstream_group: "上游分组",
  upstream_group_id: "上游分组 ID",
  upstream_group_name: "上游分组",
  upstream_key_already_absent: "上游 Key 原本不存在",
  upstream_key_created: "上游 Key 已创建",
  upstream_key_delete_requested: "上游 Key 已请求删除",
  upstream_key_deleted: "上游 Key 已删除",
  upstream_key_id: "上游 Key ID",
  upstream_key_projection_deleted: "上游 Key 本地记录已清理",
  upstream_key_readback_failed: "上游 Key 写后复核失败",
  upstream_key_secret_deleted: "上游 Key 密文已删除",
  upstream_key_still_readable: "上游 Key 仍可读取",
  usable_rounds: "有效检测轮数",
  verified: "已复验数量",
  local_projection_deleted: "本地账号记录已清理",
  pending: "待续状态",
  schedulable_warning: "调度状态警告",
  account_commit_unknown: "管理账号提交结果未知",
  key_commit_unknown: "上游 Key 提交结果未知",
  cancel_reason: "取消原因",
  recovered: "恢复成功数量",
  success: "是否成功",
  transient: "是否临时故障",
  code: "结果代码",
  auth_method: "鉴权方式",
  trigger_status: "触发状态",
  interaction_kind: "交互类型",
  refresh_attempt: "刷新尝试",
  refresh_kind: "刷新方式",
  balance_status: "余额读取状态",
  display_balance: "显示余额",
  balance_unit: "余额单位",
  checker: "检测器",
  protocol: "请求协议",
  claimed_model: "声明模型",
  verdict: "检测结论",
  similarity_percent: "相似度",
  coverage: "有效响应覆盖",
  parseable: "可解析数量",
  percent: "覆盖率",
  evidence_coverage_percent: "有效证据覆盖率",
  requests: "请求统计",
  successful: "成功请求数量",
  elapsed_seconds: "耗时",
  scope: "检测范围",
  source_name: "来源名称",
  items: "执行明细",
  requested: "请求数量",
  updated: "已更新",
  unchanged: "无需更新",
  missing: "未绑定/缺失",
  fallback: "降级读取",
  before: "变更前",
  after: "变更后",
  name_before: "原账号名称",
  name_after: "新账号名称",
  upstream_host: "上游地址",
  upstream_raw_multiplier: "上游原始倍率",
  recharge_rate: "充值比例",
  account_multiplier: "账号成本倍率",
  observation_source: "倍率来源",
  probe_error: "探测异常",
  abnormal: "计费异常数量",
  account_cost: "账号计费金额",
  actual_cost: "实际收费金额",
  attribution_level: "归因级别",
  auth_status: "鉴权状态",
  backup_id: "备份 ID",
  captcha_challenge: "验证码挑战",
  category: "核算分类",
  challenge_id: "挑战 ID",
  comparable: "可精确核对数量",
  current_group_ids: "当前分组 ID",
  decisions: "分配明细",
  desired_group_ids: "目标分组 ID",
  difference: "计费差额",
  eligible_groups: "可选分组",
  expires_at: "过期时间",
  generated_at: "生成时间",
  group: "分组",
  id: "记录 ID",
  image_data: "验证码图片",
  issues: "问题明细",
  local_group: "本地分组",
  local_groups: "本地分组",
  local_sync: "本地同步结果",
  management_base_url: "管理端 Base URL",
  name: "名称",
  note: "说明",
  report_date: "核算日期",
  request_model: "请求模型",
  results: "执行明细",
  revenue: "收入",
  rows: "核算明细",
  status_code: "HTTP 状态码",
  stored: "凭据已保存",
  summaries: "分组汇总",
  summary: "汇总",
  temporary_key: "临时密钥",
  timezone: "统计时区",
  tolerance: "允许误差",
  unavailable: "无法核对数量",
  upstream_base_url: "上游 Base URL",
  upstream_cost: "折算后上游成本",
  upstream_description: "上游说明",
  upstream_key_count: "上游 Key 数量",
  upstream_key_name: "上游 Key 名称",
  upstream_raw_cost: "上游原始成本",
  upstream_type: "上游类型",
  effective_source: "有效数据来源",
  fallback_reason: "降级原因",
  malformed_rows: "异常数据行",
  monitored_accounts: "监控账号数",
  monitoring_available: "监控数据可用",
  probe_duration_seconds: "主动探测耗时",
  probes_persisted: "已保存主动探测样本",
  requested_source: "请求的数据来源",
  source_errors: "数据源错误",
  traffic_checked: "已检查真实流量",
  traffic_duration_seconds: "真实流量耗时",
  traffic_persisted: "已保存流量样本",
  account_decisions: "账号调度决策",
  account_targets: "调度目标",
  configuration_errors: "配置错误",
  cost_tier: "成本区间",
  cost_wall: "成本墙",
  desired_concurrency: "目标并发上限",
  desired_load_factor: "目标负载因子",
  health_evaluations: "健康评估数量",
  health_score: "健康评分",
  latest_event: "最近事件",
  long_score: "长期评分",
  newly_fused: "新增熔断数量",
  rank: "分组内排名",
  rate: "当前倍率",
  rate_known: "倍率已确认",
  rate_reason: "倍率依据",
  recovery_target: "恢复阈值",
  released: "已交还控制数量",
  role: "调度角色",
  routing_state: "调度状态",
  sample_count: "样本数量",
  scaling_cooldown_active: "流量调整冷却中",
  short_score: "短期评分",
  state_since: "状态开始时间",
  survivors: "保底强留账号数量",
  ttfb_p50_ms: "P50 首字延迟",
  ttfb_p95_ms: "P95 首字延迟",
  weight: "调度权重",
  write_cooldown_active: "写入冷却中",
  abandon_control: "放弃调度控制",
  account_rate_sync: "账号倍率同步",
  alerts: "发现告警数量",
  applied: "已执行变更数量",
  auth_recovery: "鉴权恢复",
  channels: "巡检账号数量",
  cleaned_up: "已自动清理数量",
  cleanup_action: "自动处置操作",
  configuration_error: "配置错误",
  degraded: "降级账号数量",
  desired_health: "目标健康状态",
  diagnostic_detail: "诊断说明",
  diagnostic_only: "仅执行诊断",
  fused: "熔断账号数量",
  fused_until: "熔断结束时间",
  monitoring_enabled: "监控已启用",
  price_management: "价格分组调整",
  probed: "主动探测账号数量",
  release_control: "交还调度控制",
  samples: "采集样本数量",
  target_concurrency: "目标并发上限",
  target_load_factor: "目标负载因子",
  target_priority: "目标优先级",
  target_schedulable: "目标调度状态",
};

export function logKindLabel(kind: UnifiedLogKind | UnifiedLogEntry["kind"]): string {
  return kindLabels[kind];
}

export function logTitleLabel(title: string): string {
  const normalized = title.trim();
  return (
    titleLabels[normalized] ?? operationTypeLabels[normalized] ?? normalized.replaceAll("_", " ")
  );
}

export function logStatusLabel(status: string): string {
  return statusLabels[status.trim().toLowerCase()] ?? status;
}

export function logStateLabel(state: UnifiedLogState): string {
  return stateLabels[state];
}

export function logEventLevel(status: string): Exclude<UnifiedLogEventLevel, "all"> {
  const normalized = status.trim().toLowerCase();
  if (["failed", "error", "cancelled", "fused"].includes(normalized)) return "error";
  if (["warning", "warn", "partial", "degraded"].includes(normalized)) return "warning";
  return "info";
}

export function logEventLevelLabel(level: UnifiedLogEventLevel): string {
  return eventLevelLabels[level];
}

export function logSourceLabel(source: UnifiedLogEntry["source"]): string {
  return sourceLabels[source];
}

export function logStatusVariant(status: string): StatusVariant {
  const normalized = status.trim().toLowerCase();
  if (["failed", "error", "cancelled"].includes(normalized)) return "danger";
  if (["warning", "partial", "degraded"].includes(normalized)) return "warning";
  if (["succeeded", "success", "ok"].includes(normalized)) return "success";
  return "info";
}

export function formatLogDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value || "未记录";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
}

export function logDetailLabel(key: string): string {
  return detailLabels[key] ?? key.replaceAll("_", " ");
}

function formatBooleanValue(key: string | undefined, value: boolean): string {
  if (key === "schedulable") return value ? "启用" : "停用";
  if (key === "remote_confirmed" || key === "readback_confirmed") {
    return value ? "已确认" : "未确认";
  }
  if (key === "remote_write" || key === "writeback") {
    return value ? "已执行" : "未执行";
  }
  if (key?.endsWith("enabled")) {
    return value ? "启用" : "停用";
  }
  return value ? "是" : "否";
}

function formatScalarLogValue(value: unknown, key?: string): string {
  const text = String(value);
  const normalized = text.trim().toLowerCase();
  if (key === "operation_type") return operationTypeLabels[text] ?? logTitleLabel(text);
  if (key === "operation" || key === "task_name" || key === "event_type") {
    return logTitleLabel(text);
  }
  if (key === "phase" || key === "stage") return phaseLabels[text] ?? text;
  if (key === "skill") return skillLabels[text] ?? text;
  if (key === "source") return sourceValueLabels[text] ?? text;
  if (key === "image_data") return "已生成";
  if (key === "status" || key === "state") return statusLabels[normalized] ?? text;
  if (key === "field_name") {
    return text
      .split(",")
      .map((field) => logDetailLabel(field.trim()))
      .join("、");
  }
  return contextualValueLabels[key ?? ""]?.[normalized] ?? commonValueLabels[normalized] ?? text;
}

export function formatLogDurationSeconds(value: unknown): string {
  const duration = typeof value === "number" ? value : Number(value);
  if (!Number.isFinite(duration) || duration < 0) return `${String(value)} 秒`;
  const formattedSeconds = new Intl.NumberFormat("zh-CN", {
    maximumFractionDigits: 1,
  }).format(duration % 60);
  if (duration < 60) return `${formattedSeconds} 秒`;
  const minutes = Math.floor(duration / 60);
  return `${minutes} 分 ${formattedSeconds} 秒`;
}

export function formatLogValue(value: unknown, key?: string): string {
  if (value === null || value === undefined || value === "") return "未记录";
  if (typeof value === "boolean") return formatBooleanValue(key, value);
  if (Array.isArray(value)) {
    return value.length ? value.map((item) => formatLogValue(item, key)).join("、") : "无";
  }
  if (typeof value === "object") {
    const entries = Object.entries(value as Record<string, unknown>);
    if (!entries.length) return "无";
    return entries
      .map(([entryKey, item]) => `${logDetailLabel(entryKey)}：${formatLogValue(item, entryKey)}`)
      .join("；");
  }
  if (typeof value === "string" || typeof value === "number") {
    return formatScalarLogValue(value, key);
  }
  return String(value);
}

export type LogDetailRow = { key: string; label: string; value: string };

function detailValue(key: string, value: unknown): string {
  if (key.endsWith("_at") && typeof value === "string") return formatLogDate(value);
  if (
    (key === "duration_seconds" || key.endsWith("_duration_seconds")) &&
    value !== null &&
    value !== undefined
  ) {
    return formatLogDurationSeconds(value);
  }
  if (key === "progress" && value !== null && value !== undefined) return `${value}%`;
  return formatLogValue(value, key);
}

export function logDetailRows(details: Record<string, unknown>): LogDetailRow[] {
  return Object.entries(details)
    .filter(
      ([key, value]) =>
        !["events", "changes", "runs"].includes(key) &&
        value !== null &&
        value !== undefined &&
        value !== "",
    )
    .map(([key, value]) => ({
      key,
      label: logDetailLabel(key),
      value: detailValue(key, value),
    }));
}

export function relatedEvents(entry: UnifiedLogEntry): UnifiedLogEntry[] {
  return Array.isArray(entry.details.events)
    ? entry.details.events.filter(
        (item): item is UnifiedLogEntry =>
          typeof item === "object" && item !== null && (item as UnifiedLogEntry).kind === "event",
      )
    : [];
}

export type LogChangeRow = {
  id: string;
  object: string;
  objectId: string;
  occurredAt: string;
  groups: string[];
  operation: string;
  change: string;
  status: string;
  result: string;
};

function auditFieldNames(value: unknown): string[] {
  if (typeof value !== "string") return [];
  return value
    .split(",")
    .map((field) => field.trim())
    .filter(Boolean);
}

function auditSnapshot(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function formatAuditChange(before: unknown, after: unknown, fields: string[]): string {
  if (fields.length === 0) return "远端状态未发生变更";
  const beforeValues = auditSnapshot(before);
  const afterValues = auditSnapshot(after);
  if (!beforeValues || !afterValues) {
    return `${formatLogValue(before)} → ${formatLogValue(after)}`;
  }
  return fields
    .map(
      (field) =>
        `${logDetailLabel(field)}：${formatLogValue(beforeValues[field], field)} → ${formatLogValue(afterValues[field], field)}`,
    )
    .join("；");
}

function auditOperation(value: Record<string, unknown>): string {
  const operationType = String(value.operation_type ?? "");
  const phase = String(value.phase ?? "");
  if (operationType === "cleanup.delete" || operationType === "account.delete") return "删除账号";
  if (operationType === "account.onboarding") return "添加账号";
  if (phase === "remote-readback") return "写后复核";
  if (value.writeback === true || value.remote_confirmed === true) {
    return "更新账号";
  }
  if (value.readback_confirmed === true) return "读取并复核账号";
  return operationTypeLabels[operationType] ?? logTitleLabel(operationType || "未记录");
}

function auditGroups(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === "string" && item.trim() !== "");
}

export function relatedChanges(entry: UnifiedLogEntry): LogChangeRow[] {
  const groups = entry.kind === "change" ? [entry] : entry.details.changes;
  if (!Array.isArray(groups)) return [];
  const rows: LogChangeRow[] = [];
  for (const group of groups) {
    if (typeof group !== "object" || group === null) continue;
    const details = (group as UnifiedLogEntry).details;
    if (!details || !Array.isArray(details.changes)) continue;
    for (const change of details.changes) {
      if (typeof change !== "object" || change === null) continue;
      const value = change as Record<string, unknown>;
      const fields = auditFieldNames(value.field_name);
      const status = String(value.state ?? "unknown");
      rows.push({
        id: String(value.id ?? rows.length),
        object: String(value.object_name ?? value.object_id ?? "未记录对象"),
        objectId: String(value.object_id ?? ""),
        occurredAt: String(value.created_at ?? ""),
        groups: auditGroups(value.group_names),
        operation: auditOperation(value),
        change: formatAuditChange(value.before, value.after, fields),
        status,
        result: logStatusLabel(status),
      });
    }
  }
  return rows;
}
