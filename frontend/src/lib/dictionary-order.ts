import type { DictionaryEntry } from "@/api";

// Ordering is presentation-only: retain supported values and their original types/metadata.
export function orderByDictionary<T>(
  items: readonly T[],
  entries: readonly DictionaryEntry[] | undefined,
  valueOf: (item: T) => string,
): T[] {
  const order = new Map(
    entries?.filter((entry) => entry.enabled).map((entry, index) => [entry.value, index]),
  );
  return [...items].sort(
    (left, right) =>
      (order.get(valueOf(left)) ?? Infinity) - (order.get(valueOf(right)) ?? Infinity),
  );
}
