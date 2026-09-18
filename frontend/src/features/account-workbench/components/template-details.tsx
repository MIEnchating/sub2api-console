import type { ReactElement } from "react";
import { fingerprintLabels, subscriptionLabels } from "../constants";
import type { WorkbenchTemplate } from "../types";

export function TemplateDetails(props: { template: WorkbenchTemplate }): ReactElement {
  const config = props.template.config;
  const plan = config.credential_extras.plan_type;
  const fingerprint = config.extra.codex_fingerprint_mode;
  const mapping = config.credential_extras.model_mapping;
  const models =
    typeof mapping === "object" && mapping !== null
      ? Object.entries(mapping).map(([from, to]) => `${from} → ${String(to)}`)
      : [];
  const rows: Array<[string, string]> = [
    ["并发", String(config.concurrency)],
    ["优先级", String(config.priority)],
    ["计费倍率", config.rate_multiplier],
    ["负载因子", config.load_factor ?? "默认"],
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
    [
      "分组",
      props.template.summary.groups.map((group) => group.name || `#${group.id}`).join("、") ||
        "未分组",
    ],
    ["模型映射", models.join("、") || "不限制"],
  ];
  return (
    <dl className="grid min-w-0 grid-cols-2 gap-x-5 gap-y-3 text-sm sm:grid-cols-3">
      {rows.map(([label, value]) => (
        <div key={label} className="min-w-0">
          <dt className="text-xs text-muted-foreground">{label}</dt>
          <dd className="mt-1 wrap-anywhere">{value}</dd>
        </div>
      ))}
    </dl>
  );
}
