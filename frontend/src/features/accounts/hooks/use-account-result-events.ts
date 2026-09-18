import type { LiveResultsStatus } from "../lib/live-results-status";
import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { api } from "@/api";
import type { AccountRecentResult, AccountStatus } from "@/api";
import {
  healthTimestampSchema,
  isOlderAccountHealthSnapshot,
} from "../lib/account-health-snapshot";

const resultSchema = z.object({
  id: z.string().regex(/^[1-9]\d*$/),
  result: z.string().nullable(),
  event_type: z.string().nullable().optional(),
  score: z.number().finite().nullable().optional(),
  observed_at: z.string().nullable(),
  latency_ms: z.number().finite().nullable(),
  duration_ms: z.number().finite().nullable().optional(),
  failure_reason: z.string().nullable(),
  source: z.string(),
});
const updateSchema = z.object({ account_id: z.string(), result: resultSchema });
const healthScoreSchema = z.number().finite().min(0).max(100).nullable();
const sampleCountSchema = z.number().int().nonnegative();
const latencySchema = z.number().finite().nonnegative().nullable();
const healthSchema = z.object({
  health_score: healthScoreSchema,
  short_score: healthScoreSchema,
  long_score: healthScoreSchema,
  sample_count: sampleCountSchema,
  short_sample_count: sampleCountSchema,
  long_sample_count: sampleCountSchema,
  ttfb_p50_ms: latencySchema,
  ttfb_p95_ms: latencySchema,
  health_evaluated_at: healthTimestampSchema,
  health_evidence_at: healthTimestampSchema.nullable(),
});
type HealthSnapshot = z.infer<typeof healthSchema>;
const snapshotSchema = z.object({
  account_id: z.string(),
  results: z.array(resultSchema).max(100),
  health: healthSchema.optional(),
});
const collectionSchema = z.object({
  account_id: z.string(),
  state: z.enum(["connected", "retrying"]),
});

function mergeAccountResults(
  previous: readonly AccountRecentResult[],
  incoming: readonly AccountRecentResult[],
): AccountRecentResult[] {
  const results = new Map<string, AccountRecentResult>();
  for (const item of [...previous, ...incoming]) {
    const key = item.id ?? `${item.source}:${item.observed_at}`;
    results.set(key, item);
  }
  return [...results.values()]
    .sort((a, b) => {
      const time = (b.observed_at ?? "").localeCompare(a.observed_at ?? "");
      if (time !== 0) return time;
      return (b.id ?? "").localeCompare(a.id ?? "", undefined, { numeric: true });
    })
    .slice(0, 100);
}

export function useAccountResultEvents(accountIDs: readonly string[]): LiveResultsStatus {
  const client = useQueryClient();
  const idsKey = [...new Set(accountIDs)].sort().join(",");
  const [status, setStatus] = useState<LiveResultsStatus>("connecting");
  useEffect(() => {
    if (!idsKey) return;
    if (typeof EventSource === "undefined") {
      setStatus("unsupported");
      return;
    }
    setStatus("connecting");
    const ids = idsKey.split(",");
    const allowed = new Set(ids);
    const retrying = new Set<string>();
    let active = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const pending = new Map<
      string,
      Array<{ results: AccountRecentResult[]; health?: HealthSnapshot }>
    >();
    const source = new EventSource(api.accountResultsEventsURL(ids), { withCredentials: true });
    const flush = (): void => {
      timer = undefined;
      if (!active) return;
      client.setQueryData<AccountStatus[]>(["accounts"], (accounts) =>
        accounts?.map((account) => {
          const updates = pending.get(account.id);
          if (!updates) return account;
          let next = account;
          for (const update of updates) {
            const health = update.health;
            if (
              health &&
              isOlderAccountHealthSnapshot(health.health_evaluated_at, next.health_evaluated_at)
            )
              continue;
            next = {
              ...next,
              ...health,
              recent_results: mergeAccountResults(
                health ? [] : next.recent_results,
                update.results,
              ),
            };
          }
          return next;
        }),
      );
      pending.clear();
    };
    const merge = (id: string, results: AccountRecentResult[], health?: HealthSnapshot): void => {
      if (!active || !allowed.has(id)) return;
      const updates = pending.get(id) ?? [];
      updates.push({ results, health });
      pending.set(id, updates);
      // Bound live-update latency without repainting the table for every SSE message.
      timer ??= setTimeout(flush, 100);
    };
    const update = (event: Event): void => {
      try {
        const parsed = updateSchema.safeParse(JSON.parse((event as MessageEvent<string>).data));
        if (parsed.success) merge(parsed.data.account_id, [parsed.data.result]);
      } catch {
        /* A malformed event must not clear already collected results. */
      }
    };
    const snapshot = (event: Event): void => {
      try {
        const parsed = snapshotSchema.safeParse(JSON.parse((event as MessageEvent<string>).data));
        if (parsed.success) merge(parsed.data.account_id, parsed.data.results, parsed.data.health);
      } catch {
        /* Reconnection will fetch a fresh snapshot. */
      }
    };
    const collection = (event: Event): void => {
      if (!active) return;
      try {
        const parsed = collectionSchema.safeParse(JSON.parse((event as MessageEvent<string>).data));
        if (!parsed.success || !allowed.has(parsed.data.account_id)) return;
        if (parsed.data.state === "retrying") retrying.add(parsed.data.account_id);
        else retrying.delete(parsed.data.account_id);
        setStatus(retrying.size > 0 ? "retrying" : "connected");
      } catch {
        /* Keep the last known connection state. */
      }
    };
    source.onerror = () => {
      if (active) setStatus("reconnecting");
    };
    source.onopen = () => {
      if (active) setStatus("connected");
    };
    source.addEventListener("result", update);
    source.addEventListener("snapshot", snapshot);
    source.addEventListener("collection", collection);
    return () => {
      active = false;
      clearTimeout(timer);
      pending.clear();
      source.close();
    };
  }, [client, idsKey]);
  return idsKey ? status : "idle";
}
