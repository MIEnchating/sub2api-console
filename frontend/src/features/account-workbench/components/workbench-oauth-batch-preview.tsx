import { useEffect, useState, type ReactElement } from "react";
import type { WorkbenchOAuthBatchPreview } from "@/api";
import { Button } from "@/components/ui/button";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { WorkbenchOAuthBatchRows } from "./workbench-oauth-batch-rows";

export function WorkbenchOAuthBatchPreviewPanel(props: {
  preview: WorkbenchOAuthBatchPreview;
  pending: boolean;
  onStart: () => void;
  onClose: () => void;
  replacingAvailable?: number;
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
  const canStart =
    !!props.preview.id &&
    props.preview.errors.length === 0 &&
    props.preview.items.length > 0 &&
    !expired;
  const sms = props.preview.items.some((item) => !!item.sms_provider);
  return (
    <section aria-label="批量授权预览" className="min-w-0 space-y-3 rounded-lg border p-4">
      <h2 className="font-medium">批量授权预览</h2>
      {props.preview.fresh_login && <p className="text-sm">本批使用全新的官方登录会话重新授权。</p>}
      {props.preview.target && (
        <p className="text-sm wrap-anywhere">
          管理目标：{props.preview.target}；授权范围：{props.preview.items.length} 个账号。
        </p>
      )}
      {props.preview.items.length > 0 && <WorkbenchOAuthBatchRows items={props.preview.items} />}
      {props.preview.errors.length > 0 && (
        <ul aria-label="授权输入问题" className="max-h-48 overflow-auto text-sm">
          {props.preview.errors.map((item) => (
            <li key={`${item.index}-${item.message}`}>
              第 {item.index + 1} 项：{item.message}
            </li>
          ))}
        </ul>
      )}
      {sms && <p className="text-sm">本批包含自动接码，可能绑定手机号并产生供应商费用。</p>}
      {props.preview.recovery_enabled && (
        <p className="text-sm">
          已选择服务器私有保存本批输入及成功结果，原到期时间不延长。结束批次会清除恢复资料。
        </p>
      )}
      {expired && props.preview.id && (
        <p role="status" className="text-sm text-muted-foreground">
          授权预览已过期，请重新解析账号。
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button disabled={!canStart || props.pending} onClick={() => setConfirm(true)}>
          确认授权 {props.preview.items.length} 个账号
        </Button>
        <Button variant="outline" disabled={props.pending} onClick={props.onClose}>
          关闭授权预览
        </Button>
      </div>
      <ConfirmActionDialog
        open={confirm}
        title="确认批量授权"
        description={`将为 ${props.preview.target || "本地私有导出"} 逐项授权 ${props.preview.items.length} 个账号。${props.preview.fresh_login ? "确认使用全新的官方登录会话。" : ""}${props.replacingAvailable ? `原批次仍有 ${props.replacingAvailable} 个成功结果未导入；启动后将清除这些结果，取消可先完成导入或私有转换。` : ""}${sms ? "确认本批绑定接码手机号并承担供应商费用。" : ""}${props.preview.scope === "local-export" ? "授权完成后仍需预览并确认生成私有文件。" : "授权完成后仍需预览并确认导入账号。"}`}
        confirmLabel="开始批量授权"
        pending={props.pending}
        onOpenChange={setConfirm}
        onConfirm={() => {
          if (canStart) props.onStart();
        }}
      />
    </section>
  );
}
