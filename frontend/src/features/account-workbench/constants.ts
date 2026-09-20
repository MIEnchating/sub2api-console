export const workbenchTabs = [
  { id: "import", label: "导入账号" },
  { id: "accounts", label: "账号列表" },
  { id: "templates", label: "配置模板" },
  { id: "records", label: "处理记录" },
  { id: "maintenance", label: "自动维护" },
] as const;
export const subscriptionLabels: Record<string, string> = {
  free: "Free",
  plus: "Plus",
  pro: "Pro 20x",
  prolite: "Pro 5x",
  self_serve_business_prolite: "Business Premium",
};
export const fingerprintLabels: Record<string, string> = {
  off: "关闭（透传）",
  device: "仅设备",
  session: "设备+会话",
  full: "完全收敛",
};
export const accountStatusLabels: Record<string, string> = {
  active: "正常",
  error: "异常",
  inactive: "已停用",
  unknown: "未知",
};
export const workbenchKeys = { accounts: ["account-workbench", "accounts"] as const };
export const templateKeys = { library: ["account-workbench", "templates"] as const };
export const runKeys = { list: ["account-workbench", "runs"] as const };
export const maintenanceKey = ["account-workbench", "maintenance"] as const;
export const maintenanceStatusLabels: Record<string, string> = {
  healthy: "正常",
  repaired: "已恢复",
  cooldown: "冷却中",
  blocked: "已停用",
  manual: "需要处理",
};
export const maintenanceActionLabels: Record<string, string> = {
  none: "检查",
  refresh: "刷新授权",
  reauthorize: "重新授权",
};
export const maintenanceReasonLabels: Record<string, string> = {
  account_healthy: "账号状态正常",
  authorization_repaired: "授权已恢复并通过核对",
  account_deactivated: "账号已被上游停用，需人工确认",
  rate_limited: "上游限流，冷却后再检查",
  upstream_unavailable: "上游暂不可用，冷却后再检查",
  permission_denied: "权限不足，请检查账号授权",
  account_error_requires_review: "账号异常，请检查站点错误详情",
  account_changed: "账号配置已变化，请刷新后核对",
  account_busy: "账号正在被其他任务修改，稍后再检查",
  account_read_failed: "账号读取失败，请检查站点连接",
  identity_missing: "缺少稳定身份，请重新导入账号",
  private_save_failed: "私有记录保存失败，请检查服务器存储",
  refresh_unconfirmed: "刷新结果待核对，未重复提交",
  authorization_expired: "本次维护授权资料已到期，请重新导入账号",
  manual_login_required: "需要人工登录，请通过导入页重新授权",
  upload_unconfirmed: "新授权待上传核对，未再次登录",
  verification_failed: "恢复检测未通过，请查看账号检测结果",
  recovery_unconfirmed: "调度恢复结果待核对，请检查站点状态",
};
export const inputKindLabels: Record<string, string> = {
  login: "登录资料",
  refresh_token: "刷新令牌",
  sub2api_json: "Sub2API JSON",
  codex_json: "Codex JSON",
};
export const runStatusLabels: Record<string, string> = {
  queued: "排队中",
  running: "处理中",
  completed: "已完成",
  needs_attention: "需要处理",
  interrupted: "已中断",
  failed: "失败",
  exported: "JSON 已就绪",
  preparing: "准备中",
  authorizing: "授权中",
  waiting_input: "等待验证",
  checking: "检测中",
  review: "待复核",
};
export const checkVerdictLabels: Record<string, string> = {
  SOL_CONSISTENT: "检测通过",
  MISMATCH: "检测不匹配",
  INCONCLUSIVE: "证据不足",
  ERROR: "检测出错",
  MATCH: "检测匹配",
  LUNA_LIKE: "更接近 Luna",
  TERRA_LIKE: "更接近 Terra",
  LUNA_CONSISTENT: "更接近 Luna",
  TERRA_CONSISTENT: "更接近 Terra",
};

export const loginInputLabels = {
  password: "账号密码",
  email_code: "邮箱验证码",
  totp_code: "2FA 验证码",
} as const;
