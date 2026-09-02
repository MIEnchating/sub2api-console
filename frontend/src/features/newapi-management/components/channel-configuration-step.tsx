import { RadioTower, RefreshCw } from "lucide-react";

import { MultiSelect } from "@/components/multi-select";
import { Button } from "@/components/ui/button";

type Props = {
  channelName: string;
  sub2APIBaseURL: string;
  newAPIGroupOptions: Array<{ value: string; label: string }>;
  selectedGroups: string[];
  selectedModelCount: number;
  modelError?: string;
  groupError?: string;
  pending: boolean;
  fetchingModels: boolean;
  onFetchModels: () => void;
  onGroupsChange: (groups: string[]) => void;
};

export function NewAPIChannelConfigurationStep(props: Props) {
  return (
    <>
      <div className="grid gap-px overflow-hidden rounded-md border bg-border sm:grid-cols-3">
        <div className="bg-background p-3">
          <p className="text-muted-foreground text-xs">渠道名称</p>
          <p className="mt-1 truncate text-sm font-medium">{props.channelName}</p>
        </div>
        <div className="bg-background p-3">
          <p className="text-muted-foreground text-xs">类型</p>
          <p className="mt-1 text-sm font-medium">Sub2API</p>
        </div>
        <div className="bg-background p-3">
          <p className="text-muted-foreground text-xs">API 地址</p>
          <p className="mt-1 truncate text-sm font-medium">{props.sub2APIBaseURL}</p>
        </div>
      </div>

      <div className="grid gap-1.5 text-sm">
        <span className="font-medium">模型</span>
        <div className="flex min-h-9 flex-wrap items-center gap-3 rounded-lg border px-3 py-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={props.fetchingModels}
            onClick={props.onFetchModels}
          >
            <RefreshCw className={props.fetchingModels ? "animate-spin" : ""} aria-hidden="true" />
            {props.fetchingModels ? "正在获取" : "从上游获取"}
          </Button>
          <span className="text-muted-foreground text-xs">
            {props.selectedModelCount > 0
              ? `已选择 ${props.selectedModelCount} 个模型`
              : "尚未选择模型"}
          </span>
        </div>
        {props.modelError ? (
          <span className="text-destructive text-xs">{props.modelError}</span>
        ) : null}
      </div>

      <div className="grid gap-1.5 text-sm">
        <span className="font-medium">New API 分组</span>
        <MultiSelect
          options={props.newAPIGroupOptions}
          selected={props.selectedGroups}
          onChange={props.onGroupsChange}
          title="选择 New API 分组"
          searchPlaceholder="搜索 New API 分组"
          clearText="清空分组"
          ariaLabel="New API 分组"
          maxVisibleChips={6}
          disabled={props.newAPIGroupOptions.length === 0}
        />
        {props.groupError ? (
          <span className="text-destructive text-xs">{props.groupError}</span>
        ) : null}
      </div>

      <div className="flex justify-end">
        <Button type="submit" disabled={props.pending || props.newAPIGroupOptions.length === 0}>
          <RadioTower aria-hidden="true" />
          {props.pending ? "正在添加" : "添加渠道"}
        </Button>
      </div>
    </>
  );
}
