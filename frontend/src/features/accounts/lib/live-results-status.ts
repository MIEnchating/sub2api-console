export const liveResultsPresentation = {
  idle: { label: "", detail: "" },
  connecting: { label: "正在连接实时请求", detail: "连接后自动追加新请求，无需刷新页面。" },
  connected: {
    label: "实时请求已连接",
    detail: "实时更新当前页账号的请求结果与健康评分，无需刷新页面。",
  },
  retrying: { label: "部分账号采集失败", detail: "正在重试上游日志采集，已有请求结果仍保留。" },
  reconnecting: {
    label: "实时请求连接中断",
    detail: "正在自动重连，已有请求结果仍保留；持续失败时请检查网络或刷新页面。",
  },
  unsupported: { label: "实时连接不可用", detail: "浏览器不支持实时连接，请手动刷新查看新请求。" },
} as const;
export type LiveResultsStatus = keyof typeof liveResultsPresentation;
