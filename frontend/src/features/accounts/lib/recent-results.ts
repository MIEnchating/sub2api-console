import type { AccountRecentResult } from "@/api";
import { healthTimestampOrder } from "./account-health-snapshot";

const resultIDOrder = new Intl.Collator(undefined, { numeric: true });

export function mergeAccountResults(
  previous: readonly AccountRecentResult[],
  incoming: readonly AccountRecentResult[],
): AccountRecentResult[] {
  const results = new Map<string, AccountRecentResult>();
  for (const item of [...previous, ...incoming]) {
    const key = item.id ?? `${item.source}:${item.observed_at}`;
    results.set(key, item);
  }
  return (
    [...results.values()]
      // Normalize once per result, preserving precision beyond Date's milliseconds.
      .map((result) => ({ result, time: healthTimestampOrder(result.observed_at ?? "") }))
      .sort((a, b) => {
        if (a.time !== b.time) return a.time < b.time ? 1 : -1;
        // Numeric collation keeps large IDs exact without rebuilding a collator
        // for every comparison in same-second request bursts.
        return resultIDOrder.compare(b.result.id ?? "", a.result.id ?? "");
      })
      .slice(0, 100)
      .map((entry) => entry.result)
  );
}
