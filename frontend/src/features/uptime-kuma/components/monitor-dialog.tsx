import type { Task } from "@/api";
import { OperationDialogFooter } from "./operation-dialog-footer";
import { notifyOperationError } from "@/lib/operation-feedback";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useEffect } from "react";
import { ApiError, type KumaMonitor, type KumaTemplate } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { monitorSchema, defaultMonitorOptions, type MonitorValues } from "../lib/schemas";
import { Save } from "lucide-react";
import { MonitorOptionsForm } from "./monitor-options-form";
import { MonitorBasicsForm } from "./monitor-basics-form";
import { MonitorRequestForm } from "./monitor-request-form";
import { TemplateSelector } from "./template-selector";
import { MonitorAuthForm } from "./monitor-auth-form";

export function MonitorDialog(props: {
  monitor: KumaMonitor | null;
  initialType?: "group";
  monitors: KumaMonitor[];
  templates?: KumaTemplate[];
  templatesPending?: boolean;
  pending: boolean;
  task?: Pick<Task, "message" | "progress"> | null;
  error?: Error | null;
  onClose: () => void;
  onSubmit: (values: MonitorValues) => void;
}) {
  const form = useForm<MonitorValues>({
    resolver: zodResolver(monitorSchema),
    defaultValues: {
      name: props.monitor?.name ?? "",
      type: props.monitor?.type ?? props.initialType ?? "http",
      template_id: props.monitor?.template_id ?? "",
      template_model: props.monitor?.template_model ?? "",
      template_clear: false,
      template_retain: !!props.monitor?.template_id,
      template_revision: props.monitor?.template_revision ?? 0,
      template_auth_override: false,
      options: {
        ...defaultMonitorOptions,
        ...props.monitor?.options,
        headers: "",
        body: "",
        auth_username: "",
        auth_password: "",
      },
      url: "",
      interval: props.monitor?.interval ?? 60,
      parent: props.monitor?.parent ?? null,
    },
  });
  const type = form.watch("type");
  const isGroup = type === "group";
  const entity = isGroup ? "分组" : "监控项";
  const templates =
    props.monitor?.template_id &&
    !props.templates?.some((item) => item.id === props.monitor?.template_id)
      ? [
          ...(props.templates ?? []),
          {
            id: props.monitor.template_id,
            revision: props.monitor.template_revision ?? 0,
            name: props.monitor.template_name ?? "已关联模板（已删除）",
            method: props.monitor.options?.method ?? "POST",
            auth_method: "none",
            headers_configured: !!props.monitor.options?.headers_configured,
            body_configured: !!props.monitor.options?.body_configured,
            auth_configured: !!props.monitor.options?.auth_configured,
            model: props.monitor.template_model ?? "",
            body_encoding: props.monitor.template_body_encoding ?? "",
          },
        ]
      : (props.templates ?? []);
  useEffect(() => {
    if (!props.error) return;
    const fields: Record<string, keyof MonitorValues> = {
      kuma_invalid_monitor: "name",
      kuma_invalid_interval: "interval",
      kuma_invalid_monitor_url: "url",
      kuma_invalid_parent: "parent",
      kuma_invalid_template_model: "template_model",
    };
    const field = props.error instanceof ApiError ? fields[props.error.code] : undefined;
    if (field) form.setError(field, { message: props.error.message });
    else notifyOperationError(props.error, "保存失败，请重试");
  }, [form, props.error]);
  const onSubmit = (values: MonitorValues): void => {
    if (
      !["http", "keyword"].includes(values.type) &&
      !templates.find((item) => item.id === values.template_id)?.monitoring
    ) {
      values.template_id = "";
      values.template_model = "";
      values.template_clear = true;
      values.template_retain = false;
      values.template_revision = 0;
      values.template_auth_override = false;
    }
    if (
      !props.monitor &&
      ["http", "keyword"].includes(values.type) &&
      !values.url &&
      !templates.find((item) => item.id === values.template_id)?.url_configured
    ) {
      form.setError("url", { message: "请输入监控地址" });
      return;
    }
    if (values.type === "group") {
      values.template_id = "";
      values.template_model = "";
      values.template_clear = true;
      values.template_retain = false;
      values.template_revision = 0;
      values.template_auth_override = false;
      values.template_settings_override = false;
      values.options = undefined;
      values.url = "";
    }
    props.onSubmit(values);
  };
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !props.pending) props.onClose();
      }}
    >
      <DialogContent
        width={isGroup ? "medium" : "wide"}
        showCloseButton={!props.pending}
        className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      >
        <DialogHeader>
          <DialogTitle>{`${props.monitor ? "编辑" : "新增"}${entity}`}</DialogTitle>
          <DialogDescription>
            {isGroup && "设置分组名称和所属分组。"}
            {!isGroup &&
              (props.monitor
                ? "编辑监控参数和所属分组。敏感字段留空保留，勾选清空后才会删除已有配置。"
                : "选择监控类型并配置检测与请求参数。")}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <form id="kuma-monitor" onSubmit={form.handleSubmit(onSubmit)}>
            <fieldset disabled={props.pending} className="grid min-w-0 gap-6">
              <MonitorBasicsForm
                form={form}
                monitor={props.monitor}
                monitors={props.monitors}
                groupOnly={props.initialType === "group" || props.monitor?.type === "group"}
                pending={props.pending}
              />
              {!isGroup && (
                <TemplateSelector
                  form={form}
                  templates={templates}
                  disabled={props.pending || !!props.templatesPending}
                  editing={!!props.monitor}
                  monitor={props.monitor}
                />
              )}
              {["http", "keyword"].includes(type) && (
                <>
                  <MonitorRequestForm form={form} monitor={props.monitor} pending={props.pending} />
                  <MonitorAuthForm form={form} monitor={props.monitor} pending={props.pending} />
                </>
              )}
              {!isGroup && (
                <MonitorOptionsForm form={form} monitor={props.monitor} pending={props.pending} />
              )}
            </fieldset>
          </form>
        </DialogBody>
        <OperationDialogFooter pending={props.pending} task={props.task}>
          <Button variant="outline" disabled={props.pending} onClick={props.onClose}>
            取消
          </Button>
          <Button type="submit" form="kuma-monitor" disabled={props.pending}>
            <Save aria-hidden="true" />
            {props.pending ? "正在保存…" : `保存${entity}`}
          </Button>
        </OperationDialogFooter>
      </DialogContent>
    </Dialog>
  );
}
