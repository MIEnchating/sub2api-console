import type { Task, WorkbenchConfig, WorkbenchOAuthSession, WorkbenchOAuthBatch } from "@/api";

export const workbenchKeys = {
  root: ["account-workbench"] as const,
  templates: ["account-workbench", "templates"] as const,
  history: ["account-workbench", "history"] as const,
  profiles: ["account-workbench", "profiles"] as const,
  smsReceipts: ["account-workbench", "sms-receipts"] as const,
  checkpoints: ["account-workbench", "oauth-checkpoints"] as const,
  queueRecoveries: ["account-workbench", "queue-recoveries"] as const,
  maintenance: ["account-workbench", "maintenance"] as const,
  exports: ["account-workbench", "exports"] as const,
  localExports: ["account-workbench", "local-exports"] as const,
  task: (id: string) => ["account-workbench", "task", id] as const,
  oauth: (id: string | null) => ["account-workbench", "oauth", id] as const,
  batch: (id: string | null) => ["account-workbench", "oauth-batch", id] as const,
  run: (id: string | null) => ["account-workbench", "run", id] as const,
};
export const workbenchTabs = [
  { id: "import", label: "导入账号" },
  { id: "mixed", label: "混合运行" },
  { id: "oauth", label: "授权登录" },
  { id: "templates", label: "配置模板" },
  { id: "exports", label: "私有导出" },
  { id: "local", label: "本地导出" },
  { id: "security", label: "账号安全" },
  { id: "history", label: "处理记录" },
  { id: "maintenance", label: "自动维护" },
] as const;
export type WorkbenchTab = (typeof workbenchTabs)[number]["id"];
export const oauthStatusLabels: Record<WorkbenchOAuthSession["status"], string> = {
  starting: "正在启动授权浏览器",
  waiting: "等待完成授权登录",
  checkpointing: "正在保存授权检查点",
  verifying: "正在验证授权结果",
  authorized: "授权成功",
  failed: "授权失败",
  cancelled: "授权已取消",
  expired: "授权会话已过期",
};
export const oauthPollingStatuses = new Set<WorkbenchOAuthSession["status"]>([
  "starting",
  "waiting",
  "checkpointing",
  "verifying",
]);
export const maxInputBytes = 2 * 1024 * 1024;
export const batchStatusLabels: Record<WorkbenchOAuthBatch["status"], string> = {
  queued: "等待批量授权",
  running: "正在逐项授权",
  authorized: "授权结束，等待导入",
  failed: "批量授权失败",
  cancelled: "批量授权已结束",
};
export const batchRowStatusLabels = {
  queued: "等待授权",
  running: "正在授权",
  succeeded: "授权成功",
  failed: "授权失败",
  cancelled: "已取消",
};
export const previewActionLabels = { check: "重新检测", reconcile: "只读核对" } as const;
export const defaultConfig: WorkbenchConfig = {
  concurrency: 10,
  priority: 0,
  rate_multiplier: "1",
  group_ids: [],
  auto_pause_on_expired: true,
};
export const taskStatusLabels: Record<Task["status"], string> = {
  queued: "等待执行",
  running: "正在处理",
  waiting_input: "等待操作",
  succeeded: "处理完成",
  partial: "部分完成",
  failed: "处理失败",
  cancelled: "已取消",
};
export const workbenchOperationLabels: Record<string, string> = {
  "account-workbench-cleanup": "账号关联资料清理",
  "account-workbench-import": "账号导入",
  "account-workbench-retry": "重新处理",
  "account-workbench-oauth": "授权登录",
  "account-workbench-oauth-recovery": "恢复授权登录",
  "account-workbench-oauth-batch": "批量授权",
  "account-workbench-export": "线上账号导出",
  "account-workbench-convert": "输入转换",
  "account-workbench-profile-export": "登录资料导出",
  "account-workbench-regenerate": "重新生成授权",
  "account-workbench-mixed": "混合运行",
  "account-workbench-security": "账号安全",
  "account-workbench-security-password": "设置账号密码",
  "account-workbench-security-totp": "启用账号双重验证",
  "account-workbench-security-batch": "批量账号安全",
  "account-workbench-maintenance": "自动维护",
};

export const cleanupKindLabels = {
  login_profile: "登录资料",
  execution: "导入检查点",
  history: "处理记录",
  account_export: "私有账号文件",
  profile_export: "私有登录资料文件",
  security_result: "账号安全结果",
};
export const maintenanceUploadStatusLabels = {
  pending: "等待上传",
  running: "正在处理",
  cooldown: "等待冷却",
  waiting_session: "等待连接会话",
  review: "待人工核对",
};
export const smsReceiptActionLabels = {
  acquire: "申请号码",
  ready: "开始接码",
  complete: "结束订单",
  release: "释放号码",
};
export const smsReceiptStateLabels = {
  submitted: "已提交，待核对",
  confirmed: "已确认",
  uncertain: "结果待核对",
};
export const checkpointStatusLabels = {
  watching: "自动保存中",
  saving: "正在保存",
  ready: "已暂停",
  restoring: "正在恢复",
  restored: "已恢复",
  failed: "需重新授权",
  deleting: "正在删除",
};
export const queueItemStatusLabels: Record<string, string> = {
  queued: "等待执行",
  running: "进行中，恢复前核对检查点",
  waiting_input: "等待验证",
  succeeded: "结果已保存",
  failed: "需核对",
  cancelled: "已停止",
};
export const resultStatusLabels: Record<string, string> = {
  waiting_input: "等待人工验证",
  review: "待人工核对",
  queued: "等待执行",
  running: "正在处理",
  updated: "已更新凭据",
  created: "已创建",
  imported: "已导入",
  skipped: "已跳过",
  duplicate: "重复账号",
  failed: "处理失败",
  succeeded: "处理完成",
  success: "处理成功",
  repaired: "已修复",
  healthy: "健康",
  unchanged: "无需处理",
  cancelled: "已取消",
  checked: "已检查",
};
