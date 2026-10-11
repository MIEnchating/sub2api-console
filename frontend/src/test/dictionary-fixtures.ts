import type { QueryClient } from "@tanstack/react-query";
import type { DictionaryEntry, DictionaryKind } from "@/api";

type Entry = { value: string; name: string };
export const standardDictionaryFixtures: Partial<Record<DictionaryKind, Entry[]>> = {
  account_type: [
    {
      value: "apikey",
      name: "API Key",
    },
    {
      value: "oauth",
      name: "OAuth",
    },
    {
      value: "sub2api",
      name: "Sub2API",
    },
    {
      value: "newapi",
      name: "New API",
    },
    {
      value: "oneapi",
      name: "OneAPI",
    },
  ],
  upstream_type: [
    {
      value: "sub2api",
      name: "Sub2API",
    },
    {
      value: "newapi",
      name: "New API",
    },
    {
      value: "oneapi",
      name: "OneAPI",
    },
    {
      value: "custom",
      name: "自定义上游",
    },
    {
      value: "apikey",
      name: "API Key",
    },
  ],
  auth_status: [
    {
      value: "已鉴权",
      name: "已鉴权",
    },
    {
      value: "已恢复",
      name: "已恢复",
    },
    {
      value: "待验证",
      name: "待验证",
    },
    {
      value: "未确认",
      name: "未确认",
    },
    {
      value: "恢复暂时失败",
      name: "恢复暂时失败",
    },
    {
      value: "鉴权失效",
      name: "鉴权失效",
    },
    {
      value: "配置错误",
      name: "配置错误",
    },
  ],
  scheduling_strategy: [
    {
      value: "balanced",
      name: "均衡",
    },
    {
      value: "price_first",
      name: "价格优先",
    },
    {
      value: "speed_first",
      name: "速度优先",
    },
    {
      value: "reliability",
      name: "稳定优先",
    },
  ],
  task_status: [
    {
      value: "queued",
      name: "排队中",
    },
    {
      value: "running",
      name: "进行中",
    },
    {
      value: "waiting_input",
      name: "等待输入",
    },
    {
      value: "succeeded",
      name: "已成功",
    },
    {
      value: "partial",
      name: "部分完成",
    },
    {
      value: "failed",
      name: "已失败",
    },
    {
      value: "cancelled",
      name: "已取消",
    },
  ],
  account_status: [
    {
      value: "manual_priority",
      name: "手动控制",
    },
    {
      value: "healthy",
      name: "健康",
    },
    {
      value: "degraded",
      name: "降级",
    },
    {
      value: "cost_blocked",
      name: "成本墙拦截",
    },
    {
      value: "concurrency_limited",
      name: "等待并发额度",
    },
    {
      value: "fused",
      name: "已熔断",
    },
    {
      value: "manual_fused",
      name: "人工熔断",
    },
    {
      value: "survivor",
      name: "保底强留",
    },
    {
      value: "paused",
      name: "已暂停",
    },
    {
      value: "disabled",
      name: "已停用",
    },
    {
      value: "excluded",
      name: "已排除",
    },
    {
      value: "unknown",
      name: "待探测",
    },
  ],
  alert_status: [
    {
      value: "firing",
      name: "告警中",
    },
    {
      value: "recovered",
      name: "已恢复",
    },
    {
      value: "suppressed",
      name: "规则已停用",
    },
    {
      value: "closed",
      name: "已关闭",
    },
  ],
  kuma_monitor_type: [
    {
      value: "http",
      name: "HTTP(S)",
    },
    {
      value: "keyword",
      name: "HTTP(S) 关键字",
    },
    {
      value: "port",
      name: "TCP 端口",
    },
    {
      value: "ping",
      name: "Ping",
    },
    {
      value: "dns",
      name: "DNS",
    },
    {
      value: "push",
      name: "Push 推送",
    },
    {
      value: "group",
      name: "分组",
    },
  ],
  platform: [
    { value: "composite", name: "Composite" },
    {
      value: "openai",
      name: "OpenAI",
    },
    {
      value: "anthropic",
      name: "Anthropic",
    },
    {
      value: "gemini",
      name: "Gemini",
    },
    {
      value: "antigravity",
      name: "Antigravity",
    },
  ],
};

export function dictionaryItems(kind: DictionaryKind, items: readonly Entry[]): DictionaryEntry[] {
  return items.map((item, index) => ({
    ...item,
    id: `${kind}-${item.value}`,
    kind,
    description: "",
    enabled: true,
    sort_order: index,
    version: 1,
    created_at: "2026-10-10T00:00:00Z",
    updated_at: "2026-10-10T00:00:00Z",
  }));
}

export function seedDictionaries(
  client: QueryClient,
  extra: Partial<Record<DictionaryKind, Entry[]>> = {},
): void {
  for (const [key, values] of Object.entries({ ...standardDictionaryFixtures, ...extra })) {
    const kind = key as DictionaryKind;
    client.setQueryData(["dictionaries", kind], { items: dictionaryItems(kind, values) });
  }
}
