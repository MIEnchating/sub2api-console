import type { DictionaryEntry } from "@/api";

// Only registered, enabled values may be offered. Keep stable IDs and business metadata.
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
  return items
    .filter((item) => order.has(valueOf(item)))
    .sort(
      (left, right) =>
        (order.get(valueOf(left)) ?? Infinity) - (order.get(valueOf(right)) ?? Infinity),
    );
}

// Display names come from the dictionary; IDs and operation-specific metadata stay local.
export function dictionaryItemName<T>(item: T, name: string): T {
  if (!name || typeof item !== "object" || item === null || Array.isArray(item)) return item;
  if ("name" in item) return { ...item, name };
  if ("label" in item) return { ...item, label: name };
  return item;
}

// Resource tables are not option lists: keep real records visible for maintenance.
export function sortRecordsByDictionary<T>(
  items: readonly T[],
  entries: readonly DictionaryEntry[] | undefined,
  valueOf: (item: T) => string,
): T[] {
  const order = new Map((entries ?? []).map((entry, index) => [entry.value, index]));
  return [...items].sort(
    (left, right) =>
      (order.get(valueOf(left)) ?? Infinity) - (order.get(valueOf(right)) ?? Infinity),
  );
}
