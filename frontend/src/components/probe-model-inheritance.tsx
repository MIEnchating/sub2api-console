import { useQuery } from "@tanstack/react-query";
import type { ReactElement } from "react";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { inheritedProbeDescription, probeFallbackLabel } from "./probe-model-inheritance/display";

export function ProbeModelInheritance(props: {
  groupIds: (string | null)[];
  metadata?: Record<string, unknown>;
}): ReactElement {
  const policy = useQuery({ queryKey: ["policy"], queryFn: api.policy });
  const groups = useQuery({
    queryKey: ["groups"],
    queryFn: api.groups,
    enabled: props.groupIds.length > 0,
  });
  const loading =
    (!policy.data && policy.isPending) ||
    (props.groupIds.length > 0 && !groups.data && groups.isPending);
  if (loading) return <ContentLoading compact label="正在读取继承的探活配置" />;
  if (!policy.data?.available || (props.groupIds.length > 0 && !groups.data)) {
    return (
      <ContentRetry
        onRetry={() => {
          void policy.refetch();
          if (props.groupIds.length) void groups.refetch();
        }}
        pending={policy.isFetching || groups.isFetching}
      />
    );
  }
  const fallback = probeFallbackLabel(props.metadata);
  const groupIds = [...new Set(props.groupIds)];
  return (
    <div
      role="note"
      aria-label="继承的探活配置"
      className="text-muted-foreground grid min-w-0 gap-1 text-xs break-all"
    >
      {groupIds.length === 0 ? (
        <p>{inheritedProbeDescription(policy.data.probe_model, fallback)}</p>
      ) : (
        groupIds.map((id) => {
          const group = groups.data?.find((item) => item.id === id && id !== null);
          if (!group)
            return (
              <p key={id ?? "missing"}>
                分组 {id ?? "ID 缺失"}：尚未取得该分组配置，请刷新分组后确认继承模型。
              </p>
            );
          if (
            group.strategy_source === "configuration_error" ||
            group.participation_status === "configuration_error"
          )
            return <p key={id}>分组「{group.name}」配置待修复，暂无法确认继承模型。</p>;
          return (
            <p key={id}>
              分组「{group.name}」：
              {inheritedProbeDescription(policy.data!.probe_model, fallback, group)}
            </p>
          );
        })
      )}
    </div>
  );
}
