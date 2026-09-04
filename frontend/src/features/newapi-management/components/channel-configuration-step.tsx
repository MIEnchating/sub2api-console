import { useMemo } from "react";
import { RadioTower, RefreshCw } from "lucide-react";

import { MultiSelect } from "@/components/multi-select";
import type { NewAPIChannelEndpoint } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const customBaseURLValue = "__custom_base_url__";

type Props = {
  channelName: string;
  sub2APIBaseURL: string;
  apiEndpoints: NewAPIChannelEndpoint[];
  baseURL: string;
  customBaseURL: boolean;
  newAPIGroupOptions: Array<{ value: string; label: string }>;
  selectedGroups: string[];
  selectedModelCount: number;
  modelError?: string;
  baseURLError?: string;
  groupError?: string;
  pending: boolean;
  fetchingModels: boolean;
  onFetchModels: () => void;
  onBaseURLModeChange: (custom: boolean) => void;
  onBaseURLChange: (baseURL: string) => void;
  onGroupsChange: (groups: string[]) => void;
};

export function NewAPIChannelConfigurationStep(props: Props) {
  const endpoints = useMemo(
    () =>
      props.apiEndpoints.length
        ? props.apiEndpoints
        : [
            {
              name: "管理平台地址",
              base_url: props.sub2APIBaseURL,
              default: true,
            },
          ],
    [props.apiEndpoints, props.sub2APIBaseURL],
  );
  const endpointLabels = useMemo(
    () =>
      new Map(
        endpoints.map((endpoint) => [endpoint.base_url, `${endpoint.name} · ${endpoint.base_url}`]),
      ),
    [endpoints],
  );

  return (
    <>
      <div className="grid gap-px border-b bg-border sm:grid-cols-2">
        <div className="bg-background px-4 py-3 sm:px-5">
          <p className="text-muted-foreground text-xs">渠道名称</p>
          <p className="mt-1 truncate text-sm font-medium">{props.channelName}</p>
        </div>
        <div className="bg-background px-4 py-3 sm:px-5">
          <p className="text-muted-foreground text-xs">类型</p>
          <p className="mt-1 text-sm font-medium">Sub2API</p>
        </div>
      </div>

      <div
        data-channel-configuration-layout=""
        className="grid min-w-0 divide-y lg:grid-cols-[minmax(0,1.35fr)_minmax(18rem,0.65fr)] lg:divide-x lg:divide-y-0"
      >
        <div className="grid min-w-0 content-start gap-5 p-4 sm:p-5">
          <div className="grid gap-1.5 text-sm">
            <label className="font-medium" htmlFor="newapi-channel-base-url-source">
              API 地址
            </label>
            <Select
              value={props.customBaseURL ? customBaseURLValue : props.baseURL || null}
              itemToStringLabel={(value) => {
                if (value === customBaseURLValue) return "自定义地址";
                return endpointLabels.get(value) ?? value;
              }}
              onValueChange={(value) => {
                if (!value) return;
                if (value === customBaseURLValue) {
                  props.onBaseURLModeChange(true);
                  return;
                }
                props.onBaseURLModeChange(false);
                props.onBaseURLChange(value);
              }}
            >
              <SelectTrigger
                id="newapi-channel-base-url-source"
                aria-label="API 地址来源"
                aria-invalid={Boolean(props.baseURLError)}
              >
                <SelectValue placeholder="选择 API 地址" />
              </SelectTrigger>
              <SelectContent>
                {endpoints.map((endpoint) => (
                  <SelectItem key={endpoint.base_url} value={endpoint.base_url}>
                    {endpoint.name} · {endpoint.base_url}
                  </SelectItem>
                ))}
                <SelectItem value={customBaseURLValue}>自定义地址</SelectItem>
              </SelectContent>
            </Select>
            {props.customBaseURL ? (
              <Input
                aria-label="自定义 API 地址"
                value={props.baseURL}
                onChange={(event) => props.onBaseURLChange(event.target.value)}
                aria-invalid={Boolean(props.baseURLError)}
                placeholder="https://api.example.com"
              />
            ) : null}
            {props.baseURLError ? (
              <span className="text-destructive text-xs">{props.baseURLError}</span>
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
        </div>

        <div className="bg-muted/10 grid min-w-0 content-start gap-1.5 p-4 text-sm sm:p-5">
          <span className="font-medium">模型</span>
          <div className="bg-background flex min-h-24 flex-col items-start justify-center gap-3 rounded-lg border border-dashed px-4 py-3">
            <span className="text-muted-foreground text-xs">
              {props.selectedModelCount > 0
                ? `已选择 ${props.selectedModelCount} 个模型`
                : "尚未选择模型"}
            </span>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={props.fetchingModels}
              onClick={props.onFetchModels}
            >
              <RefreshCw
                className={props.fetchingModels ? "animate-spin" : ""}
                aria-hidden="true"
              />
              {props.fetchingModels ? "正在获取" : "从上游获取"}
            </Button>
          </div>
          {props.modelError ? (
            <span className="text-destructive text-xs">{props.modelError}</span>
          ) : null}
        </div>
      </div>

      <div className="bg-muted/20 flex justify-end border-t px-4 py-3 sm:px-5">
        <Button type="submit" disabled={props.pending || props.newAPIGroupOptions.length === 0}>
          <RadioTower aria-hidden="true" />
          {props.pending ? "正在添加" : "添加渠道"}
        </Button>
      </div>
    </>
  );
}
