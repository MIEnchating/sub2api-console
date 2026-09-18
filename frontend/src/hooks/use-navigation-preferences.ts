import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api";
import {
  readHiddenNavigationItemIDs,
  writeHiddenNavigationItemIDs,
} from "@/lib/navigation-preferences";
import { browserPreferenceStorage } from "@/lib/browser-preferences";
import { notifyOperationError } from "@/lib/operation-feedback";

const queryKey = ["navigation-preferences"] as const;

export function useNavigationPreferences<T extends string>(
  enabled: boolean,
  allowed: readonly T[],
  locked: ReadonlySet<T>,
) {
  const client = useQueryClient();
  const [local] = useState(() =>
    readHiddenNavigationItemIDs(browserPreferenceStorage, allowed, [...locked]),
  );
  const initialized = useRef(false);
  const writing = useRef(false);
  const query = useQuery({ queryKey, queryFn: api.navigationPreferences, enabled, retry: false });
  const save = useMutation({
    mutationFn: api.saveNavigationPreferences,
    onSuccess: (data) => {
      client.setQueryData(queryKey, data);
      writeHiddenNavigationItemIDs(
        browserPreferenceStorage,
        new Set(data.hidden_item_ids ?? []),
        allowed,
      );
    },
    onError: (error) => {
      notifyOperationError(error, "菜单设置保存失败，请重试");
      void client.invalidateQueries({ queryKey });
    },
    onSettled: () => {
      writing.current = false;
    },
  });
  const mutate = save.mutate;
  useEffect(() => {
    if (!enabled) {
      initialized.current = false;
      return;
    }
    if (query.data?.hidden_item_ids !== null || initialized.current) return;
    initialized.current = true;
    writing.current = true;
    mutate({ hidden_item_ids: [...local], version: query.data.version });
  }, [enabled, local, mutate, query.data]);
  const hiddenNavigationItemIDs = useMemo(() => {
    const values = query.data?.hidden_item_ids ?? [...local];
    return new Set(allowed.filter((id) => values.includes(id) && !locked.has(id)));
  }, [allowed, local, locked, query.data]);
  const navigationPending =
    !enabled || query.isPending || query.isFetching || query.isError || save.isPending;
  function persist(ids: Set<T>): void {
    if (navigationPending || writing.current || !query.data) return;
    writing.current = true;
    mutate({ hidden_item_ids: [...ids], version: query.data.version });
  }
  return {
    hiddenNavigationItemIDs,
    navigationPending,
    navigationLoading: query.isPending,
    navigationReadFailed: query.isError,
    retryNavigation: () => {
      void query.refetch();
    },
    setNavigationItemVisibility: (id: T, visible: boolean) => {
      if (locked.has(id)) return;
      const next = new Set(hiddenNavigationItemIDs);
      if (visible) next.delete(id);
      else next.add(id);
      persist(next);
    },
    resetNavigation: () => persist(new Set<T>()),
  };
}
