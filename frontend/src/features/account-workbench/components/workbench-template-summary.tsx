import type { ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type WorkbenchConfig, type WorkbenchTemplateInput } from "@/api";

export function WorkbenchTemplateSummary(props: {
  config: WorkbenchConfig;
  match?: WorkbenchTemplateInput["match"];
}): ReactElement {
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
  const rows: [string, string | number][] = [
    [
      "分组",
      props.config.group_ids
        ?.map((id) => groups.data?.find((item) => item.id === id)?.name ?? `ID ${id}`)
        .join("、") || "未分组",
    ],
    ["代理", props.config.proxy_id ? `ID ${props.config.proxy_id}` : "直连"],
    ["并发", props.config.concurrency ?? 10],
    ["调度优先级", props.config.priority ?? 0],
    ["计费倍率", props.config.rate_multiplier ?? "1"],
    ["负载因子", props.config.load_factor ?? "默认"],
    ["套餐匹配", props.match?.plan_type || "通用"],
    ["模型映射", Object.keys(props.config.credential_extras?.model_mapping ?? {}).length],
    ["附加设置", Object.keys(props.config.extra ?? {}).length],
  ];
  return (
    <dl
      aria-label="配置摘要"
      className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 border-t pt-4 text-sm"
    >
      {rows.map(([label, value]) => (
        <div key={label} className="contents">
          <dt className="text-muted-foreground">{label}</dt>
          <dd className="min-w-0 wrap-anywhere">{value}</dd>
        </div>
      ))}
    </dl>
  );
}
