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
  "automatic-inspection": "自动巡检",
  "manual-inspection": "手动巡检",
  "recover-host": "鉴权恢复",
  "management-snapshot-sync": "账号与分组同步",
  "upstream-balances-sync": "上游余额同步",
  "upstream-groups-sync": "上游分组同步",
  "balance-sync": "余额同步",
  "upstream-sync": "上游同步",
  "upstream-delete": "删除上游",
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
  probe_model_rewritten: "探测模型被改写",
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
};

const operationTypeLabels: Record<string, string> = {
  "routing.writeback": "自动调度写回",
  "routing.restore": "交还调度控制",
  "routing.writeback.batch": "批量自动调度",
  "account.scheduling": "账号调度",
  "account.sync": "账号字段同步",
  "account.groups.sync": "账号分组同步",
  "account.onboarding": "添加账号",
  "cleanup.delete": "自动删除账号",
  "upstream.delete": "删除上游",
  "upstream.rate_sync": "上游倍率同步",
  "notifications.test": "通知测试",
};

const phaseLabels: Record<string, string> = {
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
};

const skillLabels: Record<string, string> = {
  console: "控制台",
  "sub2api-auto-inspection": "自动巡检",
  "sub2api-upstream-auth": "上游鉴权",
  "sub2api-upstream-info": "上游信息同步",
  "sub2api-connectivity-test": "连通性探测",
  "sub2api-operations": "账号与分组管理",
};

const sourceValueLabels: Record<string, string> = {
  console: "控制台",
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

function detailLabel(key: string): string {
  return detailLabels[key] ?? key.replaceAll("_", " ");
}

function formatBooleanValue(key: string | undefined, value: boolean): string {
  if (key === "schedulable") return value ? "启用" : "停用";
  if (key === "remote_confirmed" || key === "readback_confirmed") {
    return value ? "已确认" : "未确认";
  }
  if (key?.endsWith("enabled") || key === "remote_write" || key === "writeback") {
    return value ? "启用" : "停用";
  }
  return value ? "是" : "否";
}

function formatScalarLogValue(value: unknown, key?: string): string {
  const text = String(value);
  if (key === "operation_type") return operationTypeLabels[text] ?? logTitleLabel(text);
  if (key === "operation" || key === "task_name" || key === "event_type") {
    return logTitleLabel(text);
  }
  if (key === "phase" || key === "stage") return phaseLabels[text] ?? text;
  if (key === "skill") return skillLabels[text] ?? text;
  if (key === "source") return sourceValueLabels[text] ?? text;
  if (key === "status" || key === "state") return statusLabels[text.toLowerCase()] ?? text;
  if (key === "field_name") {
    return text
      .split(",")
      .map((field) => detailLabel(field.trim()))
      .join("、");
  }
  return commonValueLabels[text.toLowerCase()] ?? text;
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
      .map(([key, item]) => `${detailLabel(key)}：${formatLogValue(item, key)}`)
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
  if (key === "duration_seconds" && value !== null && value !== undefined) return `${value} 秒`;
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
      label: detailLabel(key),
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
        `${detailLabel(field)}：${formatLogValue(beforeValues[field], field)} → ${formatLogValue(afterValues[field], field)}`,
    )
    .join("；");
}

function auditOperation(value: Record<string, unknown>): string {
  const operationType = String(value.operation_type ?? "");
  const phase = String(value.phase ?? "");
  if (operationType === "cleanup.delete") return "删除账号";
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
