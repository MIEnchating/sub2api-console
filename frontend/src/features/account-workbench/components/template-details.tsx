import type { ReactElement } from "react";
import { cn } from "@/lib/utils";
import { fingerprintLabels, subscriptionLabels } from "../constants";
import type { WorkbenchTemplate } from "../types";
import { TemplateModelMapping } from "./template-model-mapping";

export function TemplateDetails(props: {
  template: WorkbenchTemplate;
  compact?: boolean;
}): ReactElement {
  const config = props.template.config;
  const plan = config.credential_extras.plan_type;
  const fingerprint = config.extra.codex_fingerprint_mode;
  const metrics: Array<[string, string]> = [
    ["并发", String(config.concurrency)],
    ["优先级", String(config.priority)],
    ["计费倍率", config.rate_multiplier],
    ["负载因子", config.load_factor ?? "默认"],
  ];
  const rows: Array<[string, string]> = [
    [
      "代理",
      props.template.summary.proxy_name || (config.proxy_id ? `代理 #${config.proxy_id}` : "直连"),
    ],
    ["订阅", typeof plan === "string" ? subscriptionLabels[plan] || "其他订阅" : "自动识别"],
    [
      "Codex 指纹",
      typeof fingerprint === "string"
        ? fingerprintLabels[fingerprint] || "其他指纹模式"
        : fingerprintLabels.off,
    ],
  ];
  return (
    <div
      className={cn(
        "grid min-w-0 items-start gap-4",
        props.compact && "@min-[48rem]:grid-cols-2 @min-[48rem]:gap-x-6",
      )}
    >
      <dl className="grid min-w-0 grid-cols-2 gap-3 rounded-lg bg-muted/40 p-3 sm:grid-cols-4">
        {metrics.map(([label, value]) => (
          <div key={label} className="min-w-0">
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd className="mt-1 text-sm font-medium tabular-nums wrap-anywhere">{value}</dd>
          </div>
        ))}
      </dl>
      <dl className="grid min-w-0 grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-3">
        {rows.map(([label, value]) => (
          <div key={label} className="min-w-0">
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd className="mt-1 wrap-anywhere">{value}</dd>
          </div>
        ))}
        <div className="col-span-full min-w-0">
          <dt className="text-xs text-muted-foreground">分组</dt>
          <dd className="mt-1.5 flex flex-wrap gap-1.5">
            {props.template.summary.groups.length === 0
              ? "未分组"
              : props.template.summary.groups.map((group) => (
                  <span
                    key={group.id}
                    className="max-w-full rounded-md border bg-muted/20 px-2 py-0.5 text-xs wrap-anywhere"
                  >
                    {group.name || `#${group.id}`}
                  </span>
                ))}
          </dd>
        </div>
      </dl>
      <TemplateModelMapping
        mapping={config.credential_extras.model_mapping}
        compact={props.compact}
      />
    </div>
  );
}
