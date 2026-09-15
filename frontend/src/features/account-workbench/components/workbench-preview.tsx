import { useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type Task, type WorkbenchPreview } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { previewActionLabel, summarizePreview } from "../lib/preview-summary";
import { notifyOperationError } from "@/lib/operation-feedback";
import { WorkbenchTask } from "./workbench-task";
import { workbenchKeys } from "../constants";

type WorkbenchPreviewPanelProps = {
  preview: WorkbenchPreview;
  pending: boolean;
  onConfirm: () => void;
  onDiscard: () => void;
  onConverted?: (task: Task) => void;
  onConversionPendingChange?: (pending: boolean) => void;
};

export function WorkbenchPreviewPanel(props: WorkbenchPreviewPanelProps): ReactElement {
  return <WorkbenchPreviewContent key={props.preview.id} {...props} />;
}

function WorkbenchPreviewContent(props: WorkbenchPreviewPanelProps): ReactElement {
  const [confirm, setConfirm] = useState(false);
  const [exportConfirm, setExportConfirm] = useState(false);
  const mounted = useRef(true);
  const client = useQueryClient();
  const conversion = useMutation({
    gcTime: 0,
    mutationFn: () => api.convertWorkbenchInput(props.preview.id),
    onSuccess: (task) => {
      if (mounted.current) {
        setExportConfirm(false);
        props.onConverted?.(task);
      }
      toast.success("私有转换任务已创建");
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
      void client.invalidateQueries({
        queryKey:
          props.preview.scope === "local-export"
            ? workbenchKeys.localExports
            : workbenchKeys.exports,
      });
    },
    onError: (error) => {
      notifyOperationError(error, "私有转换未开始，请重新预览后重试");
      if (mounted.current) {
        setExportConfirm(false);
        props.onDiscard();
      }
    },
  });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    props.onConversionPendingChange?.(conversion.isPending);
    return () => props.onConversionPendingChange?.(false);
  }, [conversion.isPending, props.onConversionPendingChange]);
  const [expired, setExpired] = useState(() => Date.parse(props.preview.expires_at) <= Date.now());
  useEffect(() => {
    const remaining = Date.parse(props.preview.expires_at) - Date.now();
    setExpired(!Number.isFinite(remaining) || remaining <= 0);
    if (!Number.isFinite(remaining) || remaining <= 0) return;
    const timer = setTimeout(() => {
      setExpired(true);
      setConfirm(false);
      setExportConfirm(false);
    }, remaining);
    return () => clearTimeout(timer);
  }, [props.preview.expires_at]);
  const importable = props.preview.items;
  const summary = summarizePreview(props.preview);
  const consumed = !!conversion.data || conversion.isError;
  const busy = props.pending || conversion.isPending;
  const exportOnly = props.preview.export_only === true;
  const refreshCount = props.preview.items.filter((item) => item.refresh_required).length;
  return (
    <section
      aria-label={exportOnly ? "账号私有转换预览" : "账号导入预览"}
      className="min-w-0 space-y-3 rounded-lg border bg-card p-4"
    >
      <h2 className="font-medium">{summary.title}</h2>
      <p className="text-sm wrap-anywhere">{summary.scope}</p>
      <p className="text-sm text-muted-foreground">{summary.detection}</p>
      <p className="text-sm text-muted-foreground">{summary.notice}</p>
      <Table
        aria-label="账号预览"
        className="min-w-[42rem]"
        containerClassName="max-h-80 overflow-auto"
      >
        <TableHeader>
          <TableRow>
            <TableHead>账号</TableHead>
            <TableHead>套餐</TableHead>
            <TableHead>配置模板</TableHead>
            <TableHead>分组 ID</TableHead>
            <TableHead>状态</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.preview.items.map((item) => (
            <TableRow key={item.id}>
              <TableCell overflowTooltip={false} className="whitespace-normal wrap-anywhere">
                {item.name || item.email || `第 ${item.index + 1} 项`}
                {item.refresh_required ? (
                  <span className="block text-xs text-muted-foreground">
                    确认后刷新并核对官方身份
                  </span>
                ) : null}
              </TableCell>
              <TableCell>{item.plan_type || "未识别"}</TableCell>
              <TableCell overflowTooltip={false} className="whitespace-normal wrap-anywhere">
                {item.template_name || "默认配置"}
              </TableCell>
              <TableCell>{item.group_ids.join("、") || "未分组"}</TableCell>
              <TableCell>{exportOnly ? "转换到私有文件" : previewActionLabel(item)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {props.preview.errors.length > 0 && (
        <ul
          className="max-h-36 overflow-y-auto text-sm text-muted-foreground"
          aria-label="未识别条目"
        >
          {props.preview.errors.map((error) => (
            <li key={error.index}>
              第 {error.index + 1} 项：{error.message}
            </li>
          ))}
        </ul>
      )}
      {expired && (
        <p role="status" className="text-sm text-muted-foreground">
          预览已过期，请重新解析账号内容。
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        {!exportOnly ? (
          <Button
            disabled={busy || consumed || expired || importable.length === 0}
            onClick={() => setConfirm(true)}
          >
            {summary.retry ? "确认重新处理" : "确认导入"} {importable.length} 个账号
          </Button>
        ) : null}
        {!summary.retry ? (
          <Button
            variant={exportOnly ? "default" : "outline"}
            disabled={
              busy ||
              consumed ||
              expired ||
              importable.length === 0 ||
              props.preview.errors.length > 0
            }
            onClick={() => setExportConfirm(true)}
          >
            生成私有 JSON 文件
          </Button>
        ) : null}
        <Button variant="outline" disabled={busy} onClick={props.onDiscard}>
          关闭预览
        </Button>
      </div>
      <ConfirmActionDialog
        open={confirm}
        title={summary.retry ? "确认重新处理账号" : "确认批量导入账号"}
        confirmLabel={summary.retry ? "创建重试任务" : "创建导入任务"}
        description={summary.description}
        pending={busy}
        onOpenChange={setConfirm}
        onConfirm={() => {
          if (!expired && !busy && !consumed && !exportOnly) props.onConfirm();
        }}
      />
      <ConfirmActionDialog
        open={exportConfirm}
        title="确认生成私有账号文件"
        confirmLabel="创建私有转换任务"
        description={
          (refreshCount > 0
            ? `其中 ${refreshCount} 项将向官方刷新 RT，旧令牌可能失效；成功项立即写入私有文件，失败项不会自动重放。`
            : "") +
          (props.preview.scope === "local-export"
            ? `将上述 ${props.preview.items.length} 项账号保留输入配置并保存为服务器私有 JSON 文件，不创建或修改线上账号，也不执行模型检测。文件保留 24 小时。`
            : `将上述 ${props.preview.items.length} 项账号按已显示的模板配置保存为服务器私有 JSON 文件，不创建或修改线上账号，也不执行模型检测。文件保留 24 小时，可在私有导出页查看产物信息。`)
        }
        pending={conversion.isPending}
        onOpenChange={setExportConfirm}
        onConfirm={() => {
          if (!expired && !consumed && !props.pending) conversion.mutate();
        }}
      />
      {conversion.data && !props.onConverted ? <WorkbenchTask task={conversion.data} /> : null}
    </section>
  );
}
