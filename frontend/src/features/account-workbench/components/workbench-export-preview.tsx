import { useEffect, useState, type ReactElement } from "react";
import type { WorkbenchExportPreview } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { Button } from "@/components/ui/button";

export function WorkbenchExportPreviewPanel(props: {
  preview: WorkbenchExportPreview;
  pending: boolean;
  onConfirm: () => void;
  onDiscard: () => void;
}): ReactElement {
  const [confirm, setConfirm] = useState(false);
  const [expired, setExpired] = useState(false);
  useEffect(() => {
    const remaining = Date.parse(props.preview.expires_at) - Date.now();
    setExpired(!Number.isFinite(remaining) || remaining <= 0);
    if (!Number.isFinite(remaining) || remaining <= 0) return;
    const timer = setTimeout(() => {
      setExpired(true);
      setConfirm(false);
    }, remaining);
    return () => clearTimeout(timer);
  }, [props.preview.expires_at]);
  return (
    <section aria-label="账号导出预览" className="grid min-w-0 gap-3 border-t pt-4">
      <h2 className="text-base font-medium">导出预览</h2>
      <p className="text-sm wrap-anywhere">管理目标：{props.preview.target}</p>
      <ul className="max-h-64 overflow-y-auto text-sm" aria-label="导出账号范围">
        {props.preview.items.map((item) => (
          <li key={item.account_id} className="py-1 wrap-anywhere">
            {item.name}（ID {item.account_id}）
          </li>
        ))}
      </ul>
      {expired && (
        <p role="status" className="text-sm">
          导出预览已过期，请重新预览。
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          disabled={props.pending || expired || !props.preview.items.length}
          onClick={() => setConfirm(true)}
        >
          确认导出 {props.preview.items.length} 个账号
        </Button>
        <Button variant="outline" disabled={props.pending} onClick={props.onDiscard}>
          关闭预览
        </Button>
      </div>
      <ConfirmActionDialog
        open={confirm}
        title="确认生成私有账号文件"
        confirmLabel="创建导出任务"
        description={`从 ${props.preview.target} 导出 ${props.preview.items.length} 个账号，ID：${props.preview.items.map((item) => item.account_id).join("、")}。含凭据的文件仅保存在后端私有目录，24 小时后过期。`}
        pending={props.pending}
        onOpenChange={setConfirm}
        onConfirm={() => {
          if (!expired) props.onConfirm();
        }}
      />
    </section>
  );
}
