export const accountStatusDictionary = [
  { value: "manual_fused", name: "人工熔断" },
  { value: "degraded", name: "降级" },
  { value: "healthy", name: "健康" },
  { value: "manual_priority", name: "手动控制" },
  { value: "cost_blocked", name: "成本墙拦截" },
  { value: "concurrency_limited", name: "等待并发额度" },
  { value: "fused", name: "已熔断" },
  { value: "survivor", name: "保底强留" },
  { value: "paused", name: "已暂停" },
  { value: "disabled", name: "已停用" },
  { value: "excluded", name: "已排除" },
  { value: "unknown", name: "待探测" },
];
