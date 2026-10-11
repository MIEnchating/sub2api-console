import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { NewAPIChannel } from "@/api";
import { QueryErrorToast } from "@/components/query-error-toast";
import { readCommonChannelModels } from "../lib/channel-model-discovery";
import { NewAPIChannelModelDialog } from "./channel-model-dialog";

export function ChannelModelDiscovery(props: {
  platformId: string;
  channels: NewAPIChannel[];
  onClose: () => void;
  onConfirm: (models: string[]) => void;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const models = useQuery({
    queryKey: [
      "newapi-channel-model-discovery",
      props.platformId,
      props.channels.map((channel) => [channel.id, channel.version]),
    ],
    queryFn: ({ signal }) => readCommonChannelModels(props.platformId, props.channels, signal),
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
  });
  const available = useMemo(
    () =>
      (models.data ?? []).filter((model) =>
        props.channels.some((channel) => !channel.models.includes(model)),
      ),
    [models.data, props.channels],
  );
  const selection = selected.filter((model) => available.includes(model));
  return (
    <>
      {models.isError && (
        <QueryErrorToast error={models.error} fallback="获取渠道上游模型失败，请重试" />
      )}
      <NewAPIChannelModelDialog
        open
        models={available}
        selected={selection}
        description={
          props.channels.length > 1
            ? `所选 ${props.channels.length} 个渠道共同支持的模型`
            : `渠道：${props.channels[0]?.name ?? ""}`
        }
        emptyText="没有可新增的上游模型"
        pending={models.isFetching}
        error={models.isError ? "获取失败" : ""}
        onOpenChange={(open) => {
          if (!open) props.onClose();
        }}
        onSelectedChange={setSelected}
        onRetry={() => void models.refetch()}
        onConfirm={() => props.onConfirm(selection)}
      />
    </>
  );
}
