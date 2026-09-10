import type { Task } from "@/api";
import { OperationDialogFooter } from "./operation-dialog-footer";
import { notifyOperationError } from "@/lib/operation-feedback";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Save } from "lucide-react";
import type { KumaResource, KumaResourceKind, KumaResourceList } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { resourceDefaults, resourceSchema, type ResourceValues } from "../lib/resource-schemas";
import { resourceTitles } from "../constants";
import { NotificationForm } from "./notification-form";
import { MaintenanceForm } from "./maintenance-form";
import { StatusPageForm } from "./status-page-form";
export function ResourceDialog(props: {
  kind: KumaResourceKind;
  item?: KumaResource;
  options: KumaResourceList;
  pending: boolean;
  task?: Pick<Task, "message" | "progress"> | null;
  error: Error | null;
  onClose: () => void;
  onSubmit: (v: ResourceValues) => void;
}) {
  const form = useForm<ResourceValues>({
    resolver: zodResolver(resourceSchema),
    defaultValues: resourceDefaults(props.kind, props.item),
  });
  useEffect(() => {
    if (props.error) notifyOperationError(props.error, "保存失败，请重试");
  }, [form, props.error]);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !props.pending) props.onClose();
      }}
    >
      <DialogContent
        width="wide"
        showCloseButton={!props.pending}
        className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      >
        <DialogHeader>
          <DialogTitle>
            {props.item ? "编辑" : "新增"}
            {resourceTitles[props.kind]}
          </DialogTitle>
          <DialogDescription>
            {props.kind === "status-pages"
              ? "保存将更新公开状态页及展示分组。绑定域名需已指向 Uptime Kuma 服务。"
              : "保存后同步到 Uptime Kuma；已有凭据留空保留。"}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <form id="kuma-resource" onSubmit={form.handleSubmit(props.onSubmit)}>
            <fieldset disabled={props.pending} className="grid min-w-0 gap-4">
              {props.kind === "notifications" && (
                <NotificationForm form={form} editing={!!props.item} pending={props.pending} />
              )}
              {props.kind === "maintenance" && (
                <MaintenanceForm form={form} options={props.options} pending={props.pending} />
              )}
              {props.kind === "status-pages" && (
                <StatusPageForm
                  form={form}
                  options={props.options}
                  pending={props.pending}
                  editing={!!props.item}
                />
              )}
            </fieldset>
          </form>
        </DialogBody>
        <OperationDialogFooter pending={props.pending} task={props.task}>
          <Button variant="outline" disabled={props.pending} onClick={props.onClose}>
            取消
          </Button>
          <Button type="submit" form="kuma-resource" disabled={props.pending}>
            <Save aria-hidden="true" />
            {props.pending ? "正在保存…" : "保存"}
          </Button>
        </OperationDialogFooter>
      </DialogContent>
    </Dialog>
  );
}
