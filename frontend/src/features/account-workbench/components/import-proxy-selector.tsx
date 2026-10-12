import { useEffect, useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { Controller, type UseFormReturn } from "react-hook-form";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { proxyKeys } from "../constants";
import type { ImportValues } from "../lib/import-schema";

export function ImportProxySelector(props: {
  form: UseFormReturn<ImportValues>;
  busy: boolean;
  onReadyChange: (ready: boolean) => void;
}): ReactElement {
  const [enabled, setEnabled] = useState(false);
  const proxies = useQuery({
    queryKey: proxyKeys.list,
    queryFn: ({ signal }) => api.workbenchProxies(signal),
    enabled,
  });
  const selected = props.form.watch("platform_proxy_id") || "";
  const ready =
    !enabled ||
    (!!proxies.data &&
      (!selected || proxies.data.some((item) => item.id === selected && item.available)));
  useEffect(() => {
    props.onReadyChange(ready);
  }, [props.onReadyChange, ready]);
  return (
    <div className="grid min-w-0 gap-2 border-t pt-4">
      <label htmlFor="workbench-platform-proxy" className="text-sm">
        统一平台代理
      </label>
      <Controller
        name="platform_proxy_id"
        control={props.form.control}
        render={({ field }) => (
          <Select
            value={field.value || "__template__"}
            disabled={props.busy || (enabled && !proxies.data)}
            onValueChange={(value, details) => {
              if (value === null || details.reason === "none") {
                details.cancel();
                return;
              }
              field.onChange(value === "__template__" ? "" : value);
              if (value !== "__template__")
                props.form.setValue("proxy_enabled", false, { shouldValidate: true });
            }}
          >
            <SelectTrigger id="workbench-platform-proxy">
              <SelectValue>
                {selected
                  ? proxies.data?.find((item) => item.id === selected)?.name ||
                    "已选代理 #" + selected
                  : "沿用模板代理"}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__template__">沿用模板代理</SelectItem>
              {proxies.data
                ?.filter((item) => item.available)
                .map((item) => (
                  <SelectItem key={item.id} value={item.id}>
                    {item.name} · {item.protocol} · #{item.id}
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
        )}
      />
      <Button
        type="button"
        variant="outline"
        disabled={props.busy || proxies.isFetching}
        onClick={() => {
          if (enabled) void proxies.refetch();
          else setEnabled(true);
        }}
      >
        同步平台代理
      </Button>
      {enabled && proxies.isPending && <ContentLoading compact label="正在读取平台代理" />}
      {enabled && proxies.isError && !proxies.data && (
        <ContentRetry pending={proxies.isFetching} onRetry={() => void proxies.refetch()} />
      )}
      {proxies.data?.length === 0 && (
        <p className="text-xs text-muted-foreground">
          平台暂无代理，请先在 Sub2API 配置后重新同步。
        </p>
      )}
      {selected &&
        proxies.data &&
        !proxies.data.some((item) => item.id === selected && item.available) && (
          <p className="text-xs text-muted-foreground">已选代理不可用，请重新选择或清除选择。</p>
        )}
      {proxies.data
        ?.filter((item) => !item.available)
        .map((item) => (
          <p key={item.id} className="text-xs wrap-anywhere text-muted-foreground">
            {item.name}（#{item.id}）：{item.reason}
          </p>
        ))}
    </div>
  );
}
