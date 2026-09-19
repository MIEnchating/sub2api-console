import type { ReactElement } from "react";
import type { UpstreamSummary } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { FormField } from "@/components/form-field";
import { MultiSelect } from "@/components/multi-select";
import { upstreamConcurrencyPolicyLabels } from "../constants";

export type AllocationUpstreamProps = {
  upstreamIDs?: string[];
  onUpstreamIDsChange?: (ids: string[]) => void;
  upstreams?: Pick<
    UpstreamSummary["hosts"][number],
    "upstream_id" | "name" | "host" | "upstream_type"
  >[];
  upstreamsPending?: boolean;
  upstreamsFailed?: boolean;
  onRetryUpstreams?: () => void;
};

export function UpstreamConcurrencyUpstreams(props: AllocationUpstreamProps): ReactElement {
  const selected = props.upstreamIDs ?? [];
  const options = new Map(
    (props.upstreams ?? [])
      .filter(
        (upstream) => upstream.upstream_type.toLowerCase() === "sub2api" && upstream.upstream_id,
      )
      .map((upstream) => [
        upstream.upstream_id,
        {
          value: upstream.upstream_id,
          label: `${upstream.name || upstream.host} · ${upstream.host}`,
        },
      ]),
  );
  for (const id of selected) {
    if (!options.has(id))
      options.set(id, { value: id, label: `上游 ${id}（当前配置，列表中未找到）` });
  }
  return (
    <div className="min-w-0 space-y-2 @min-[56rem]/policy-card:col-span-2">
      <FormField label={upstreamConcurrencyPolicyLabels.upstreams}>
        <MultiSelect
          options={[...options.values()]}
          selected={selected}
          onChange={(ids) => props.onUpstreamIDsChange?.(ids)}
          title="选择上游"
          searchPlaceholder="按上游名称或地址搜索"
          emptyText="没有可选择的 Sub2API 上游"
          ariaLabel={upstreamConcurrencyPolicyLabels.upstreams}
          disabled={props.upstreamsPending || props.upstreamsFailed}
        />
      </FormField>
      {props.upstreamsPending ? <ContentLoading label="正在加载上游" compact /> : null}
      {props.upstreamsFailed && props.onRetryUpstreams ? (
        <ContentRetry onRetry={props.onRetryUpstreams} />
      ) : null}
      <p className="text-muted-foreground text-xs leading-5">
        {upstreamConcurrencyPolicyLabels.upstreamScope}
      </p>
      {selected.length === 0 ? (
        <p className="text-muted-foreground text-xs leading-5">
          {upstreamConcurrencyPolicyLabels.emptyUpstreamScope}
        </p>
      ) : null}
    </div>
  );
}
