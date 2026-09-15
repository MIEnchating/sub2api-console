import { useEffect, useId, useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { api, type WorkbenchTemplate, type WorkbenchTemplateSource } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export function WorkbenchTemplateSourcePicker(props: {
  item?: WorkbenchTemplate;
  disabled: boolean;
  onApply: (source: WorkbenchTemplateSource | null) => void;
  onBlockedChange: (blocked: boolean) => void;
}): ReactElement {
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const [sourceId, setSourceId] = useState(props.item?.source_account_id ?? "");
  const [search, setSearch] = useState("");
  const instance = useId();
  const extract = useQuery({
    queryKey: ["account-workbench", "template-source", instance, sourceId],
    queryFn: () => api.workbenchTemplateFromAccount(sourceId),
    enabled: !!sourceId,
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
  });
  const blocked = !sourceId || extract.isFetching || !extract.data || extract.isError;
  useEffect(() => props.onBlockedChange(blocked), [blocked, props.onBlockedChange]);
  useEffect(() => {
    if (extract.data && !blocked) props.onApply(extract.data);
  }, [extract.data, blocked, props.onApply]);
  const candidates = (accounts.data ?? []).filter(
    (item) => item.platform === "openai" && item.account_type === "oauth",
  );
  const selected = candidates.find((item) => item.id === sourceId);
  const filtered = candidates.filter((item) =>
    `${item.name} ${item.id} ${(item.groups ?? []).join(" ")}`
      .toLowerCase()
      .includes(search.trim().toLowerCase()),
  );
  return (
    <section aria-label="模板来源" className="min-w-0 space-y-3">
      <div className="flex min-w-0 items-center gap-2">
        <Input
          aria-label="搜索线上账号"
          placeholder="搜索名称、ID、分组"
          value={search}
          disabled={props.disabled}
          onChange={(event) => setSearch(event.target.value)}
        />
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                size="icon"
                variant="outline"
                aria-label="刷新账号列表"
                disabled={props.disabled || accounts.isFetching}
                onClick={() => void accounts.refetch()}
              />
            }
          >
            <RefreshCw aria-hidden="true" />
          </TooltipTrigger>
          <TooltipContent>刷新账号列表</TooltipContent>
        </Tooltip>
      </div>
      <FormField label="来源账号">
        <Select
          value={sourceId}
          disabled={props.disabled || !accounts.data}
          onValueChange={(value) => {
            props.onBlockedChange(true);
            props.onApply(null);
            setSourceId(value ?? "");
          }}
        >
          <SelectTrigger aria-label="来源账号">
            <SelectValue placeholder="选择 OpenAI OAuth 账号">
              {sourceId
                ? `${selected?.name ?? props.item?.source_name ?? "来源账号"}（ID ${sourceId}）`
                : undefined}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {props.item?.source_account_id &&
              !candidates.some((item) => item.id === props.item?.source_account_id) && (
                <SelectItem value={props.item.source_account_id}>
                  {props.item.source_name || "来源账号"}（ID {props.item.source_account_id}）
                </SelectItem>
              )}
            {filtered.map((item) => (
              <SelectItem key={item.id} value={item.id}>
                {item.name}（ID {item.id}）
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FormField>
      {search && !filtered.length && (
        <p className="text-sm text-muted-foreground">没有匹配的账号</p>
      )}
      {accounts.isPending && <ContentLoading label="正在读取可选账号" compact />}
      {!accounts.data && accounts.isError && (
        <ContentRetry pending={accounts.isFetching} onRetry={() => void accounts.refetch()} />
      )}
      {accounts.data && !candidates.length && !props.item?.source_account_id && (
        <p className="text-sm text-muted-foreground">暂无可提取配置的 OpenAI OAuth 账号</p>
      )}
      {extract.isFetching && <ContentLoading label="正在读取账号配置" compact />}
      {extract.isError && (
        <ContentRetry pending={extract.isFetching} onRetry={() => void extract.refetch()} />
      )}
      {extract.data && !blocked && (
        <Button
          type="button"
          variant="outline"
          disabled={props.disabled}
          onClick={() => void extract.refetch()}
        >
          <RefreshCw aria-hidden="true" />
          重新读取配置
        </Button>
      )}
    </section>
  );
}
