import type { UpstreamGroupChange } from "@/api";

export function groupHistoryTime(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

type UpstreamHistorySummary = {
  upstreamID: string;
  latestAt: string;
  added: number;
  removed: number;
  changes: UpstreamGroupChange[];
};

export function aggregateGroupHistory(rows: UpstreamGroupChange[]): UpstreamHistorySummary[] {
  const summaries = new Map<string, UpstreamHistorySummary>();
  const sorted = [...rows].sort((left, right) => {
    const leftTime = Date.parse(left.changed_at) || 0;
    const rightTime = Date.parse(right.changed_at) || 0;
    return rightTime - leftTime || right.id - left.id;
  });
  for (const row of sorted) {
    let summary = summaries.get(row.upstream_id);
    if (!summary) {
      summary = {
        upstreamID: row.upstream_id,
        latestAt: row.changed_at,
        added: 0,
        removed: 0,
        changes: [],
      };
      summaries.set(row.upstream_id, summary);
    }
    summary.changes.push(row);
    if (row.change_type === "added") summary.added += 1;
    if (row.change_type === "removed") summary.removed += 1;
  }
  return [...summaries.values()];
}
