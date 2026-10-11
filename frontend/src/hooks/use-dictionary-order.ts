import { useQuery } from "@tanstack/react-query";
import { api, type DictionaryKind } from "@/api";
import {
  dictionaryItemName,
  orderByDictionary,
  sortRecordsByDictionary,
} from "@/lib/dictionary-order";

export function useDictionary(kind: DictionaryKind) {
  return useQuery({
    queryKey: ["dictionaries", kind],
    queryFn: async () => {
      const result = await api.dictionaries(kind);
      if (!result || !Array.isArray(result.items)) {
        throw new Error("字典返回格式不正确，请重新读取");
      }
      return result;
    },
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  });
}

export function useDictionaryRecordOrder<T>(
  kind: DictionaryKind,
  items: readonly T[],
  valueOf: (item: T) => string,
): T[] {
  const query = useDictionary(kind);
  return sortRecordsByDictionary(items, query.data?.items, valueOf);
}

export function useDictionaryLabel<Value extends string>(
  kind: DictionaryKind,
  fallback: (value: Value) => string,
) {
  const query = useDictionary(kind);
  return (value: Value): string =>
    query.data?.items.find((entry) =>
      kind === "platform"
        ? entry.value.trim().toLocaleLowerCase() === value.trim().toLocaleLowerCase()
        : entry.value === value,
    )?.name ?? fallback(value);
}

export function useDictionaryOrder<T>(
  kind: DictionaryKind,
  items: readonly T[],
  valueOf: (item: T) => string,
  entryKey: "value" | "name" = "value",
): T[] {
  const query = useDictionary(kind);
  const dictionary =
    kind === "platform"
      ? query.data?.items.map((entry) => ({
          ...entry,
          value: entry.value.trim().toLocaleLowerCase(),
        }))
      : query.data?.items;
  const keyOf = (item: T): string =>
    kind === "platform" ? valueOf(item).trim().toLocaleLowerCase() : valueOf(item);
  const entries = new Map<string, string>();
  for (const entry of dictionary ?? []) {
    if (entry.enabled && !entries.has(entry[entryKey])) entries.set(entry[entryKey], entry.name);
  }
  return orderByDictionary(items, dictionary, keyOf, entryKey).map((item) =>
    dictionaryItemName(item, entries.get(keyOf(item)) ?? ""),
  );
}
