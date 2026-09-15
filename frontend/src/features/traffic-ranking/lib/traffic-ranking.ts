import type { TrafficRankingRow } from "@/api";

const tokenMillionsFormatter = new Intl.NumberFormat("zh-CN", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

export function formatTrafficCount(value: number): string {
  return new Intl.NumberFormat("zh-CN").format(value);
}

export function formatTrafficTokens(value: number | null): string {
  if (value === null) return "未提供";
  if (value > 0 && value < 10_000) return "<0.01 M";
  return `${tokenMillionsFormatter.format(value / 1_000_000)} M`;
}

export function formatTrafficCachePercent(
  usage: Pick<TrafficRankingRow, "input_tokens" | "cache_read_tokens" | "cache_write_tokens">,
  field: "cache_read_tokens" | "cache_write_tokens",
): string {
  if (
    usage.input_tokens === null ||
    usage.cache_read_tokens === null ||
    usage.cache_write_tokens === null
  ) {
    return "未提供";
  }
  const total = usage.input_tokens + usage.cache_read_tokens + usage.cache_write_tokens;
  if (total === 0) return "-";
  const cached = field === "cache_read_tokens" ? usage.cache_read_tokens : usage.cache_write_tokens;
  return formatTrafficPercent((cached / total) * 100);
}

export function formatTrafficPercent(value: number | null): string {
  return value === null ? "-" : `${value.toFixed(2)}%`;
}

export function formatTrafficLatency(value: number | null): string {
  if (value === null) return "-";
  if (value >= 1000) return `${(value / 1000).toFixed(2)} s`;
  return `${value.toFixed(value >= 100 ? 0 : 1)} ms`;
}

export function trafficStabilityLabel(value: number | null): {
  label: string;
  variant: "secondary" | "warning" | "destructive" | "outline";
} {
  if (value === null) return { label: "无样本", variant: "outline" };
  if (value >= 90) return { label: "稳定", variant: "secondary" };
  if (value >= 70) return { label: "观察", variant: "warning" };
  return { label: "不稳定", variant: "destructive" };
}

export function trafficAccountMatches(row: TrafficRankingRow, query: string): boolean {
  const normalized = query.trim().toLocaleLowerCase();
  if (normalized === "") return true;
  return [row.account_id, row.account_name, row.upstream_host, row.platform, ...row.groups].some(
    (value) => value.toLocaleLowerCase().includes(normalized),
  );
}
