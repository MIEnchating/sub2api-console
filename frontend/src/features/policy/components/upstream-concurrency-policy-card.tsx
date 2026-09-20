import { UpstreamAllocationOverrides } from "./upstream-allocation-overrides";
import type { ReactElement } from "react";
import type { AccountStatus } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { FormField } from "@/components/form-field";
import { MultiSelect } from "@/components/multi-select";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { upstreamConcurrencyPolicyLabels, upstreamConcurrencyAccountModes } from "../constants";
import type { UpstreamConcurrencyMode } from "../constants";
import {
  UpstreamConcurrencyUpstreams,
  type AllocationUpstreamProps,
} from "./upstream-concurrency-upstreams";
import { PolicyHelp } from "./policy-help";
import { PolicyConfigCard } from "./policy-config-card";

type AllocationAccount = Pick<AccountStatus, "id" | "name" | "upstream_type" | "groups">;

export function UpstreamConcurrencyPolicyCard(
  props: AllocationUpstreamProps & {
    accountOverrides?: Record<string, boolean>;
    upstreamOverrides?: Record<string, boolean>;
    onAccountOverridesChange?: (value: Record<string, boolean>) => void;
    onUpstreamOverridesChange?: (value: Record<string, boolean>) => void;
    enabled: boolean;
    onEnabledChange: (enabled: boolean) => void;
    accountMode: UpstreamConcurrencyMode;
    accountIDs: string[];
    onAccountModeChange: (mode: UpstreamConcurrencyMode) => void;
    onAccountIDsChange: (ids: string[]) => void;
    accounts?: AllocationAccount[];
    accountsPending?: boolean;
    accountsFailed?: boolean;
    onRetryAccounts?: () => void;
  },
): ReactElement {
  const options = new Map(
    (props.accounts ?? [])
      .filter((account) => account.upstream_type?.toLowerCase() === "sub2api")
      .map((account) => [
        account.id,
        {
          value: account.id,
          label: `${account.name}（#${account.id}）${account.groups.length ? ` · ${account.groups.join("、")}` : ""}`,
        },
      ]),
  );
  for (const id of props.accountIDs) {
    if (!options.has(id))
      options.set(id, { value: id, label: `账号 #${id}（当前配置，列表中未找到）` });
  }
  return (
    <PolicyConfigCard
      title={upstreamConcurrencyPolicyLabels.title}
      description={upstreamConcurrencyPolicyLabels.description}
      wide
      columns={3}
      help={
        <PolicyHelp label={upstreamConcurrencyPolicyLabels.help}>
          <p>{upstreamConcurrencyPolicyLabels.scope}</p>
          <p>{upstreamConcurrencyPolicyLabels.recovery}</p>
        </PolicyHelp>
      }
      switchAction={{
        checked: props.enabled,
        label: upstreamConcurrencyPolicyLabels.toggle,
        onCheckedChange: props.onEnabledChange,
      }}
    >
      <div className="min-w-0 space-y-2">
        <FormField label={upstreamConcurrencyPolicyLabels.accountScope}>
          <Select
            value={props.accountMode}
            onValueChange={(mode) => {
              if (mode === "all" || mode === "selected" || mode === "upstreams")
                props.onAccountModeChange(mode);
            }}
          >
            <SelectTrigger aria-label={upstreamConcurrencyPolicyLabels.accountScope}>
              <SelectValue>
                {
                  upstreamConcurrencyAccountModes.find((item) => item.value === props.accountMode)
                    ?.label
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {upstreamConcurrencyAccountModes.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FormField>
        {props.accountMode === "all" ? (
          <p className="text-muted-foreground text-xs leading-5">
            {upstreamConcurrencyPolicyLabels.allScope}
          </p>
        ) : null}
      </div>
      {props.accountMode === "selected" ? (
        <div className="min-w-0 space-y-2 @min-[56rem]/policy-card:col-span-2">
          <FormField label={upstreamConcurrencyPolicyLabels.accounts}>
            <MultiSelect
              options={[...options.values()]}
              selected={props.accountIDs}
              onChange={props.onAccountIDsChange}
              title="选择账号"
              searchPlaceholder="按账号名称、ID 或分组搜索"
              emptyText="没有可选择的 Sub2API 账号"
              ariaLabel={upstreamConcurrencyPolicyLabels.accounts}
              disabled={props.accountsPending || props.accountsFailed}
            />
          </FormField>
          {props.accountsPending ? <ContentLoading label="正在加载账号" compact /> : null}
          {props.accountsFailed && props.onRetryAccounts ? (
            <ContentRetry onRetry={props.onRetryAccounts} />
          ) : null}
          <p className="text-muted-foreground text-xs leading-5">
            {upstreamConcurrencyPolicyLabels.selectedScope}
          </p>
          {props.accountIDs.length === 0 ? (
            <p className="text-muted-foreground text-xs leading-5">
              {upstreamConcurrencyPolicyLabels.emptyScope}
            </p>
          ) : null}
        </div>
      ) : null}
      {props.accountMode === "upstreams" ? <UpstreamConcurrencyUpstreams {...props} /> : null}
      <p className="text-muted-foreground text-xs leading-5 @min-[56rem]/policy-card:col-span-3">
        账号单独设置优先于上游单独设置，再跟随上述范围；总开关控制整个共享并发分配功能。
      </p>
      <UpstreamAllocationOverrides
        label="账号单独设置"
        values={props.accountOverrides ?? {}}
        names={
          new Map((props.accounts ?? []).map((item) => [item.id, `${item.name}（#${item.id}）`]))
        }
        onChange={(value) => props.onAccountOverridesChange?.(value)}
      />
      <UpstreamAllocationOverrides
        label="上游单独设置"
        values={props.upstreamOverrides ?? {}}
        names={
          new Map((props.upstreams ?? []).map((item) => [item.upstream_id, item.name || item.host]))
        }
        onChange={(value) => props.onUpstreamOverridesChange?.(value)}
      />
    </PolicyConfigCard>
  );
}
