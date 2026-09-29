export type HistoryNavigation = {
  history?: "overview" | "host";
  history_host?: string;
  history_upstream?: string;
};

export function historyNavigation(search: Record<string, unknown>): HistoryNavigation {
  const history =
    search.history === "overview" || search.history === "host" ? search.history : undefined;
  if (!history) return {};
  return {
    history,
    history_host: typeof search.history_host === "string" ? search.history_host : undefined,
    history_upstream:
      typeof search.history_upstream === "string" ? search.history_upstream : undefined,
  };
}
