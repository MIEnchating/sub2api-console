import { useState } from "react";
import type { ReactNode } from "react";
import { Copy, Download, Loader2 } from "lucide-react";
import { toast } from "sonner";

import type { RemoteModelPricingSource } from "@/api";
import { TableActionButton } from "@/components/data-table/table-action-button";

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / (1024 * 1024)).toFixed(2)} MB`;
}

export function RawPricingSourceMetadata(props: { source: RemoteModelPricingSource }): ReactNode {
  return (
    <details className="min-w-0 text-xs">
      <summary className="text-muted-foreground cursor-pointer leading-6">
        <span className="text-foreground font-medium">文件信息</span>
        <span className="ml-3">{formatBytes(props.source.size_bytes)}</span>
        <span className="ml-3">{new Date(props.source.fetched_at).toLocaleString("zh-CN")}</span>
      </summary>
      <dl className="mt-2 grid max-h-24 gap-2 overflow-auto border-l pl-3">
        <div className="min-w-0">
          <dt className="text-muted-foreground">来源 URL</dt>
          <dd className="mt-1 font-mono [overflow-wrap:anywhere]">{props.source.source_url}</dd>
        </div>
        <div className="min-w-0">
          <dt className="text-muted-foreground">SHA-256</dt>
          <dd className="mt-1 font-mono [overflow-wrap:anywhere]">{props.source.sha256}</dd>
        </div>
      </dl>
    </details>
  );
}

export function RawPricingSourceActions(props: {
  source: RemoteModelPricingSource;
  showCopy?: boolean;
}): ReactNode {
  const [copying, setCopying] = useState(false);
  async function copy(): Promise<void> {
    setCopying(true);
    try {
      if (!navigator.clipboard) throw new Error("clipboard unavailable");
      await navigator.clipboard.writeText(props.source.content);
      toast.success("已复制原始文件");
    } catch {
      toast.error("复制失败，请允许剪贴板权限或下载原始文件");
    } finally {
      setCopying(false);
    }
  }

  function download(): void {
    let address: string | undefined;
    const link = document.createElement("a");
    try {
      address = URL.createObjectURL(
        new Blob([props.source.content], { type: "application/json;charset=utf-8" }),
      );
      link.href = address;
      link.download = "model-prices.json";
      document.body.append(link);
      link.click();
    } catch {
      toast.error("下载失败，请检查浏览器下载权限后重试");
    } finally {
      link.remove();
      if (address) URL.revokeObjectURL(address);
    }
  }

  return (
    <div className="flex shrink-0 items-center gap-2">
      {props.showCopy !== false && (
        <TableActionButton label="复制原始文件" disabled={copying} onClick={() => void copy()}>
          {copying ? (
            <Loader2 aria-hidden="true" className="animate-spin" />
          ) : (
            <Copy aria-hidden="true" />
          )}
        </TableActionButton>
      )}
      <TableActionButton label="下载原始文件" onClick={download}>
        <Download aria-hidden="true" />
      </TableActionButton>
    </div>
  );
}
