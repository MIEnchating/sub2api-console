import type { DictionaryEntry } from "@/api";

// Ordering is presentation-only: retain supported values and their original types/metadata.
export function orderByDictionary<T>(
  items: readonly T[],
  entries: readonly DictionaryEntry[] | undefined,
  valueOf: (item: T) => string,
  entryKey: "value" | "name" = "value",
): T[] {
  const order = new Map<string, number>();
  for (const entry of entries ?? []) {
    if (entry.enabled && !order.has(entry[entryKey])) {
      order.set(entry[entryKey], order.size);
    }
  }
  return [...items].sort(
    (left, right) =>
      (order.get(valueOf(left)) ?? Infinity) - (order.get(valueOf(right)) ?? Infinity),
  );
}
