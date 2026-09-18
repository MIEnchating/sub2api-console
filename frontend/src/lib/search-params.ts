import { defaultParseSearch, defaultStringifySearch } from "@tanstack/react-router";

export function parseConsoleSearch(value: string): Record<string, unknown> {
  const parsed: Record<string, unknown> = defaultParseSearch(value);
  const ids = new URLSearchParams(value).getAll("account_id");
  if (ids.length === 1 && /^[1-9]\d*$/.test(ids[0])) parsed.account_id = ids[0];
  return parsed;
}

export function stringifyConsoleSearch(search: Record<string, unknown>): string {
  if (typeof search.account_id !== "string" || !/^[1-9]\d*$/.test(search.account_id)) {
    return defaultStringifySearch(search);
  }
  const rest = { ...search };
  delete rest.account_id;
  const query = defaultStringifySearch(rest);
  return `${query}${query ? "&" : "?"}account_id=${search.account_id}`;
}
