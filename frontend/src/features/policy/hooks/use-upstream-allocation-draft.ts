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
  const [draft, setDraft] = useState<boolean | undefined>();
  const key = ["upstream-allocation", kind, id];
  const query = useQuery({
    queryKey: key,
    queryFn: () => api.upstreamAllocationSetting(kind, id),
    enabled,
  });
  useEffect(() => setDraft(undefined), [kind, id, enabled]);
  const checked = draft ?? query.data?.effective ?? false;
  const ready = !enabled || Boolean(query.data);

  async function save(): Promise<void> {
    if (!enabled) return;
    const data = query.data;
    if (!data) throw new Error(labels.refresh);
    if (draft === undefined || draft === data.effective) return;
    try {
      const value = await api.setUpstreamAllocationSetting(kind, id, {
        override: draft,
        expected_revision: data.revision,
        expected_upstream_id: data.upstream_id,
      });
      client.setQueryData(key, value);
      setDraft(undefined);
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

  return { query, checked, ready, onCheckedChange: setDraft, save };
}
