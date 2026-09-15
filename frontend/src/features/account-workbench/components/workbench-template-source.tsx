import { useEffect, useId, useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { Download, RefreshCw } from "lucide-react";
import { api, type WorkbenchTemplate, type WorkbenchTemplateSource } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { FormField } from "@/components/form-field";
import { JsonEditor } from "@/components/json-editor";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { TemplateValues } from "../lib/schemas";

type SourcePreview = { source: WorkbenchTemplateSource; previous: TemplateValues };

export function WorkbenchTemplateSourcePicker(props: {
  item?: WorkbenchTemplate;
  value: WorkbenchTemplateSource | null;
  disabled: boolean;
  refresh: boolean;
  current: () => TemplateValues;
  onApply: (source: WorkbenchTemplateSource) => void;
  onBlockedChange: (blocked: boolean) => void;
}): ReactElement {
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const [sourceId, setSourceId] = useState(props.item?.source_account_id ?? "");
  const [preview, setPreview] = useState<SourcePreview | null>(null);
  const instance = useId();
  const [requested, setRequested] = useState({
    accountId: props.refresh ? sourceId : "",
    sequence: 0,
  });
  const extract = useQuery({
    queryKey: [
      "account-workbench",
      "template-source",
      instance,
      requested.sequence,
      requested.accountId,
    ],
    queryFn: () => api.workbenchTemplateFromAccount(requested.accountId),
    enabled: !!requested.accountId,
    retry: false,
    refetchOnWindowFocus: false,
    gcTime: 0,
  });
  useEffect(() => {
    if (extract.data) setPreview({ source: extract.data, previous: props.current() });
  }, [extract.data, props.current]);
  const blocked = extract.isFetching || preview !== null;
  useEffect(() => props.onBlockedChange(blocked), [blocked, props.onBlockedChange]);
  const candidates = (accounts.data ?? []).filter(
    (item) => item.platform === "openai" && item.account_type === "oauth",
  );
  const accountId = props.value?.account_id ?? props.item?.source_account_id;
  const sourceName = props.value?.account_name ?? props.item?.source_name;
  const target = props.value?.target ?? props.item?.target_url;
  const synced = props.value?.synced_at ?? props.item?.source_synced_at;
  const disabled = props.disabled || blocked;
  const selected = candidates.find((item) => item.id === sourceId);
  const selectedName = selected?.name ?? sourceName;
  return (
    <section aria-label="模板来源" className="min-w-0 space-y-3 border-b pb-4">
      {accountId && (
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-sm">
          <dt className="text-muted-foreground">已关联来源</dt>
          <dd className="wrap-anywhere">
            {sourceName || "来源账号"}（ID {accountId}）
          </dd>
          <dt className="text-muted-foreground">管理目标</dt>
          <dd className="wrap-anywhere">{target}</dd>
          <dt className="text-muted-foreground">来源读取时间</dt>
          <dd className="wrap-anywhere">{synced || "尚未读取"}</dd>
        </dl>
      )}
      <FormField label="来源账号">
        <Select
          value={sourceId}
          onValueChange={(value) => setSourceId(value ?? "")}
          disabled={disabled || !accounts.data}
        >
          <SelectTrigger aria-label="来源账号">
            <SelectValue placeholder="选择 OpenAI OAuth 账号">
              {sourceId ? `${selectedName || "来源账号"}（ID ${sourceId}）` : undefined}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {accountId && !candidates.some((item) => item.id === accountId) && (
              <SelectItem value={accountId}>
                {sourceName || "来源账号"}（ID {accountId}）
              </SelectItem>
            )}
            {candidates.map((item) => (
              <SelectItem key={item.id} value={item.id}>
                {item.name}（ID {item.id}）
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FormField>
      {accounts.isPending && <ContentLoading label="正在读取可选账号" compact />}
      {!accounts.data && accounts.isError && (
        <ContentRetry pending={accounts.isFetching} onRetry={() => void accounts.refetch()} />
      )}
      {accounts.data && !candidates.length && !accountId && (
        <p className="text-sm text-muted-foreground">暂无可提取配置的 OpenAI OAuth 账号。</p>
      )}
      <Button
        type="button"
        variant="outline"
        disabled={!sourceId || disabled}
        onClick={() => setRequested({ accountId: sourceId, sequence: requested.sequence + 1 })}
      >
        <RefreshCw aria-hidden="true" />
        {accountId ? "预览来源更新" : "从账号读取配置"}
      </Button>
      {extract.isFetching && <ContentLoading label="正在读取账号配置" compact />}
      {extract.isError && (
        <ContentRetry pending={extract.isFetching} onRetry={() => void extract.refetch()} />
      )}
      {preview && (
        <section aria-label="来源配置预览" className="min-w-0 space-y-3">
          <p className="text-sm wrap-anywhere">
            {preview.source.account_name}（ID {preview.source.account_id}） ·{" "}
            {preview.source.target}
          </p>
          <div className="grid min-w-0 gap-3 sm:grid-cols-2">
            <div className="min-w-0 space-y-2">
              <h3 className="text-sm font-medium">当前模板配置</h3>
              <JsonEditor
                readOnly
                aria-label="当前模板配置 JSON"
                className="h-56"
                value={JSON.stringify(
                  {
                    config: preview.previous.config,
                    match: preview.previous.match,
                    priority: preview.previous.priority,
                  },
                  null,
                  2,
                )}
              />
            </div>
            <div className="min-w-0 space-y-2">
              <h3 className="text-sm font-medium">来源账号配置</h3>
              <JsonEditor
                readOnly
                aria-label="来源账号配置 JSON"
                className="h-56"
                value={JSON.stringify(
                  {
                    config: preview.source.config,
                    match: preview.source.match,
                    priority: preview.source.priority,
                  },
                  null,
                  2,
                )}
              />
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={props.disabled}
              onClick={() => setPreview(null)}
            >
              取消来源更新
            </Button>
            <Button
              type="button"
              disabled={props.disabled}
              onClick={() => {
                props.onApply(preview.source);
                setPreview(null);
              }}
            >
              <Download aria-hidden="true" />
              应用来源配置
            </Button>
          </div>
        </section>
      )}
    </section>
  );
}
