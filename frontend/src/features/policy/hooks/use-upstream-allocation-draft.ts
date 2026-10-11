import { useQuery, useQueryClient, type UseQueryResult } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, type UpstreamAllocationSetting, type UpstreamAllocationTarget } from "@/api";
import { allocationSettingLabels as labels } from "../constants";

export type UpstreamAllocationDraft = {
  query: UseQueryResult<UpstreamAllocationSetting>;
  checked: boolean;
  ready: boolean;
  onCheckedChange: (checked: boolean) => void;
  save: () => Promise<void>;
};

export function useUpstreamAllocationDraft(
  kind: UpstreamAllocationTarget,
  id: string,
  enabled: boolean,
): UpstreamAllocationDraft {
  const client = useQueryClient();
  const [draft, setDraft] = useState<{
    kind: UpstreamAllocationTarget;
    id: string;
    enabled: boolean;
    checked: boolean;
    base: UpstreamAllocationSetting;
  }>();
  const key = ["upstream-allocation", kind, id];
  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => api.upstreamAllocationSetting(kind, id, signal),
    enabled,
  });
  useEffect(() => setDraft(undefined), [kind, id, enabled]);
  const currentDraft =
    draft?.kind === kind && draft.id === id && draft.enabled === enabled ? draft : undefined;
  const checked = currentDraft?.checked ?? query.data?.effective ?? false;
  const ready = !enabled || Boolean(query.data);

  async function save(): Promise<void> {
    if (!enabled) return;
    const data = currentDraft?.base ?? query.data;
    if (!data) throw new Error(labels.refresh);
    if (!currentDraft || currentDraft.checked === data.effective) return;
    try {
      await client.cancelQueries({ queryKey: key, exact: true });
      const value = await api.setUpstreamAllocationSetting(kind, id, {
        override: currentDraft.checked,
        expected_revision: data.revision,
        expected_upstream_id: data.upstream_id,
      });
      await client.cancelQueries({ queryKey: key, exact: true });
      client.setQueryData(key, value);
      setDraft((latest) => (latest === currentDraft ? undefined : latest));
      // Keep the acknowledged value visible until the next read; invalidating an
      // active editor here could overwrite its draft during the remaining save.
      await Promise.all([
        client.invalidateQueries({ queryKey: ["upstream-allocation"], refetchType: "none" }),
        client.invalidateQueries({ queryKey: ["policy"] }),
        client.invalidateQueries({ queryKey: ["logs"] }),
      ]);
    } catch (error) {
      void client.invalidateQueries({ queryKey: key });
      throw error;
    }
  }

  const onCheckedChange = (checked: boolean): void => {
    if (!query.data) return;
    setDraft({ kind, id, enabled, checked, base: currentDraft?.base ?? query.data });
  };
  return { query, checked, ready, onCheckedChange, save };
}
