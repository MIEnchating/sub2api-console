import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type NewAPIChannel, type NewAPIChannelLookup } from "@/api";
import type { DataTablePaginationProps } from "@/components/data-table/pagination";

export type AutomationChannelPageControls = {
  pagination: DataTablePaginationProps;
  pending: boolean;
  failed: boolean;
  retry: () => void;
};

type AutomationChannelCatalog = {
  selectedIDs: string[];
  changeSelection: (ids: string[]) => void;
  channels: NewAPIChannel[];
  pageControls: AutomationChannelPageControls;
  opened: boolean;
  pending: boolean;
  ready: boolean;
  pageError: Error | null;
  selectionError: Error | null;
  refresh: () => void;
};

export function useAutomationChannelCatalog(
  platformId: string,
  initialIDs: string[],
): AutomationChannelCatalog {
  const client = useQueryClient();
  const [selectedIDs, setSelectedIDs] = useState(initialIDs);
  const [retained, setRetained] = useState<NewAPIChannel[]>([]);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const [opened, setOpened] = useState(false);
  const channels = useQuery({
    queryKey: ["newapi-channels", platformId, "automation-editor", "page", page, pageSize],
    queryFn: async ({ signal }) => {
      const result = await api.newAPIChannels(platformId, page, pageSize, "", signal);
      signal.throwIfAborted();
      if (
        result.items.length > pageSize ||
        new Set(result.items.map((item) => item.id)).size !== result.items.length
      ) {
        throw new Error("渠道分页结果不完整或重复，请重新读取");
      }
      return result;
    },
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const selection = useQuery({
    queryKey: ["newapi-channels", platformId, "automation-editor", "selection", selectedIDs],
    queryFn: ({ signal }) => api.newAPIChannelLookup(platformId, selectedIDs, signal),
    enabled: selectedIDs.length > 0,
    // A newly selected item already has a validated public row from this page.
    // Refresh/invalidation still re-reads every current selection by stable ID.
    initialData: (): NewAPIChannelLookup | undefined => {
      const known = new Map(
        [...(channels.data?.items ?? []), ...retained].map((item) => [item.id, item]),
      );
      if (!selectedIDs.every((id) => known.has(id))) return undefined;
      return { items: selectedIDs.map((id) => known.get(id)!), failures: [] };
    },
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const catalog = useMemo(() => {
    const selected = new Set(selectedIDs);
    const known = new Map(
      retained.filter((item) => selected.has(item.id)).map((item) => [item.id, item]),
    );
    const snapshots = [
      { at: channels.dataUpdatedAt, items: channels.data?.items ?? [] },
      { at: selection.dataUpdatedAt, items: selection.data?.items ?? [] },
    ].sort((a, b) => a.at - b.at);
    for (const snapshot of snapshots)
      for (const item of snapshot.items) if (selected.has(item.id)) known.set(item.id, item);
    const unavailable = new Set(selection.data?.failures.map((failure) => failure.id) ?? []);
    for (const id of unavailable) known.delete(id);
    return [
      ...known.values(),
      ...(channels.data?.items ?? []).filter(
        (item) => !selected.has(item.id) && !unavailable.has(item.id),
      ),
    ];
  }, [
    selectedIDs,
    retained,
    channels.data,
    channels.dataUpdatedAt,
    selection.data,
    selection.dataUpdatedAt,
  ]);
  const changeSelection = (ids: string[]): void => {
    const selected = new Set(ids);
    setRetained(catalog.filter((item) => selected.has(item.id)));
    setSelectedIDs(ids);
  };
  const retainSelection = (): void => {
    const selected = new Set(selectedIDs);
    const items = catalog.filter((item) => selected.has(item.id));
    setRetained(items);
    if (selection.data && !selection.isFetching && !selection.isError) {
      // A page may have a newer version than the original ID lookup. Retain
      // those observed public rows before releasing the previous page cache.
      client.setQueryData<NewAPIChannelLookup>(
        ["newapi-channels", platformId, "automation-editor", "selection", selectedIDs],
        { items, failures: selection.data.failures },
      );
    }
  };
  // Record only that the form has opened; changing pages must not unmount it.
  if (!opened && channels.data) setOpened(true);
  const pending = channels.isFetching || selection.isFetching;
  const total = channels.data?.total ?? -1;
  const items = channels.data?.items ?? [];
  const totalItems =
    total < 0 && channels.data && items.length < pageSize ? page * pageSize + items.length : total;
  const totalPages =
    totalItems < 0 ? page + 2 : Math.max(page + 1, Math.ceil(totalItems / pageSize));
  const pageControls: AutomationChannelPageControls = {
    pagination: {
      currentPage: page + 1,
      totalPages: Math.min(1000, totalPages),
      totalItems,
      pageSize,
      onPageChange: (value) => {
        retainSelection();
        setPage(value - 1);
      },
      onPageSizeChange: (value) => {
        retainSelection();
        setPage(0);
        setPageSize(value);
      },
    },
    pending: channels.isFetching,
    failed: channels.isError,
    retry: () => {
      void channels.refetch();
    },
  };
  const refresh = (): void => {
    void channels.refetch();
    if (selectedIDs.length > 0) void selection.refetch();
  };
  return {
    selectedIDs,
    changeSelection,
    channels: catalog,
    pageControls,
    opened,
    pending,
    ready: !pending && !channels.isError && !selection.isError,
    pageError: channels.error,
    selectionError: selection.error,
    refresh,
  };
}
