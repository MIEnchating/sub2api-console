import { replaceEqualDeep } from "@tanstack/react-query";
import { z } from "zod";

import type { AccountStatus } from "@/api";

export const healthTimestampSchema = z.string().datetime();

function healthTimestampOrder(value: string): string {
  // Go emits UTC RFC3339Nano with variable precision; Date truncates to milliseconds.
  return value.replace(/(?:\.(\d+))?Z$/, (_suffix, fraction: string | undefined) => {
    return `.${(fraction ?? "").padEnd(9, "0")}Z`;
  });
}

export function isOlderAccountHealthSnapshot(incoming: unknown, previous: unknown): boolean {
  const incomingTime = healthTimestampSchema.safeParse(incoming);
  const previousTime = healthTimestampSchema.safeParse(previous);
  return (
    incomingTime.success &&
    previousTime.success &&
    healthTimestampOrder(incomingTime.data) < healthTimestampOrder(previousTime.data)
  );
}

export function shareAccountHealthSnapshots(previous: unknown, incoming: unknown): unknown {
  if (!Array.isArray(previous) || !Array.isArray(incoming)) {
    return replaceEqualDeep(previous, incoming);
  }
  // This structural-sharing callback belongs only to the typed accounts query.
  const previousAccounts = new Map((previous as AccountStatus[]).map((item) => [item.id, item]));
  const accounts = (incoming as AccountStatus[]).map((account) => {
    const cached = previousAccounts.get(account.id);
    if (
      !cached ||
      cached === account ||
      !isOlderAccountHealthSnapshot(account.health_evaluated_at, cached.health_evaluated_at)
    ) {
      return account;
    }
    return {
      ...account,
      health_score: cached.health_score,
      short_score: cached.short_score,
      long_score: cached.long_score,
      sample_count: cached.sample_count,
      short_sample_count: cached.short_sample_count,
      long_sample_count: cached.long_sample_count,
      ttfb_p50_ms: cached.ttfb_p50_ms,
      ttfb_p95_ms: cached.ttfb_p95_ms,
      health_evaluated_at: cached.health_evaluated_at,
      health_evidence_at: cached.health_evidence_at,
      recent_results: cached.recent_results,
    };
  });
  return replaceEqualDeep(previous, accounts);
}
