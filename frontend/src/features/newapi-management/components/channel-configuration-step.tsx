import { useMemo } from "react";
import { FieldError } from "@/components/field-error";
import { RadioTower, RefreshCw } from "lucide-react";

import { ChannelFormColumns, ChannelFormFooter } from "./channel-form-layout";
import { Badge } from "@/components/ui/badge";
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
  modelRequestFailed?: boolean;
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
      <div className="flex min-w-0 flex-wrap items-start gap-2 border-b border-border/70 px-4 py-3">
        <div className="min-w-0 flex-1 space-y-1">
          <p className="text-xs text-muted-foreground">渠道名称</p>
          <p className="wrap-anywhere text-sm font-semibold">{props.channelName}</p>
        </div>
        <Badge variant="secondary">Sub2API</Badge>
      </div>

      <ChannelFormColumns kind="configuration">
        <div className="contents">
          <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.5 text-sm">
            <label className="font-medium" htmlFor="newapi-channel-base-url-source">
              API 地址
            </label>
            <Select
              disabled={props.pending || props.fetchingModels}
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
                className="min-w-0"
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
                disabled={props.pending || props.fetchingModels}
                value={props.baseURL}
                onChange={(event) => props.onBaseURLChange(event.target.value)}
                aria-invalid={Boolean(props.baseURLError)}
                placeholder="https://api.example.com"
              />
            ) : null}
            {props.baseURLError && <FieldError message={props.baseURLError} />}
          </div>

          <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-1.5 text-sm">
            <span className="font-medium">New API 分组</span>
            <MultiSelect
              options={props.newAPIGroupOptions}
              selected={props.selectedGroups}
              onChange={props.onGroupsChange}
              title="选择 New API 分组"
              searchPlaceholder="搜索 New API 分组"
              clearText="清空分组"
              ariaLabel="New API 分组"
              maxVisibleChips={3}
              disabled={props.pending || props.newAPIGroupOptions.length === 0}
            />
            {props.newAPIGroupOptions.length === 0 && (
              <p className="text-xs leading-5 text-muted-foreground">
                暂无可用的 New API 分组，请配置后刷新页面。
              </p>
            )}
            {props.groupError && <FieldError message={props.groupError} />}
          </div>
        </div>

        <div className="col-span-full grid min-w-0 content-start gap-1.5 border-t border-border/70 pt-4 text-sm">
          <h3 className="text-sm font-medium">模型范围</h3>
          <div className="flex min-w-0 flex-wrap items-center justify-between gap-3">
            <span role="status" className="text-sm font-medium">
              {props.selectedModelCount > 0
                ? `已选择 ${props.selectedModelCount} 个模型`
                : "尚未选择模型"}
            </span>
            <Button
              type="button"
              variant="outline"
              disabled={props.pending || props.fetchingModels}
              onClick={props.onFetchModels}
            >
              <RefreshCw
                className={props.fetchingModels ? "animate-spin motion-reduce:animate-none" : ""}
                aria-hidden="true"
              />
              {props.fetchingModels ? "正在获取" : "从上游获取"}
            </Button>
          </div>
          {props.modelError && <FieldError message={props.modelError} />}
        </div>
      </ChannelFormColumns>

      <ChannelFormFooter note="确认分组和模型范围后，添加到 New API 平台。">
        <Button
          type="submit"
          disabled={
            props.pending ||
            props.fetchingModels ||
            Boolean(props.modelError) ||
            props.modelRequestFailed ||
            props.newAPIGroupOptions.length === 0
          }
        >
          <RadioTower aria-hidden="true" />
          {props.pending ? "正在添加" : "添加渠道"}
        </Button>
      </ChannelFormFooter>
    </>
  );
}
