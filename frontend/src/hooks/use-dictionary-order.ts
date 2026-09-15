import { useQuery } from "@tanstack/react-query";
import { api, type DictionaryKind } from "@/api";
import { orderByDictionary } from "@/lib/dictionary-order";

export function useDictionaryOrder<T>(
  kind: DictionaryKind,
  items: readonly T[],
  valueOf: (item: T) => string,
  entryKey: "value" | "name" = "value",
): T[] {
  const query = useQuery({
    queryKey: ["dictionaries", kind],
    queryFn: () => api.dictionaries(kind),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  });
  return orderByDictionary(items, query.data?.items, valueOf, entryKey);
}
