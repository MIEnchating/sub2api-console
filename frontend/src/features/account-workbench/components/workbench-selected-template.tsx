import type { ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type WorkbenchTemplate } from "@/api";

export function WorkbenchSelectedTemplate(props: { template: WorkbenchTemplate }): ReactElement {
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
  const names = props.template.config.group_ids.map(
    (id) => groups.data?.find((group) => group.id === id)?.name ?? `ID ${id}`,
  );
  return (
    <div
      aria-label="已选模板摘要"
      className="grid min-w-0 gap-1 text-sm text-muted-foreground wrap-anywhere"
    >
      <span>{names.join("、") || "未分组"}</span>
      <span>
        并发 {props.template.config.concurrency} · 倍率 {props.template.config.rate_multiplier}
      </span>
    </div>
  );
}
