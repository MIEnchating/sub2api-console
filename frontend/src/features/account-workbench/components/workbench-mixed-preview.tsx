import { useEffect, useState, type ReactElement } from "react";
import type { WorkbenchRunPreview } from "@/api";
import { Button } from "@/components/ui/button";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { WorkbenchMixedRows } from "./workbench-mixed-rows";

export function WorkbenchMixedPreview(props: {
  preview: WorkbenchRunPreview;
  pending: boolean;
  onStart: () => void;
  onClose: () => void;
  confirmInitially?: boolean;
}): ReactElement {
  const [confirm, setConfirm] = useState(props.confirmInitially ?? false);
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
  const canStart =
    !!props.preview.id &&
    props.preview.items.length > 0 &&
    props.preview.errors.length === 0 &&
    !expired;
  const sms = props.preview.items.some((item) => !!item.sms_provider);
  return (
    <section aria-label="账号内容预览" className="grid min-w-0 gap-3 rounded-lg border p-4">
      <h2 className="font-medium">账号预览</h2>
      <p className="text-sm wrap-anywhere">
        管理目标：{props.preview.target}；处理方式：
        {props.preview.export_only ? "生成私有 JSON" : "导入线上账号"}；共{" "}
        {props.preview.items.length} 项。
      </p>
      <WorkbenchMixedRows rows={props.preview.items} />
      {props.preview.errors.length > 0 && (
        <ul aria-label="混合输入问题" className="max-h-48 overflow-auto text-sm">
          {props.preview.errors.map((error) => (
            <li key={`${error.index}-${error.message}`}>
              第 {error.index + 1} 项：{error.message}
            </li>
          ))}
        </ul>
      )}
      {expired && props.preview.id && (
        <p role="status" className="text-sm text-muted-foreground">
          账号内容预览已过期，请重新填写资料并预览。
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button disabled={!canStart || props.pending} onClick={() => setConfirm(true)}>
          确认处理 {props.preview.items.length} 项
        </Button>
        <Button variant="outline" disabled={props.pending} onClick={props.onClose}>
          关闭预览
        </Button>
      </div>
      <ConfirmActionDialog
        open={confirm && canStart}
        title="确认处理账号"
        description={`将在 ${props.preview.target} 准备 ${props.preview.items.length} 项账号资料，依次完成所需的凭据刷新和官方登录。${sms ? "包含自动接码，确认手机号绑定并承担供应商费用。" : ""}准备完成后还需确认${props.preview.export_only ? "生成私有 JSON 文件" : "导入线上账号"}。`}
        confirmLabel="开始处理"
        pending={props.pending}
        onOpenChange={setConfirm}
        onConfirm={() => {
          if (canStart) props.onStart();
        }}
      />
    </section>
  );
}
