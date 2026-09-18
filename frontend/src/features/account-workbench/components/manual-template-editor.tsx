import type { ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import { Textarea } from "@/components/ui/textarea";
import { JsonEditorField } from "@/components/json-editor/form-field";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { notifyOperationError } from "@/lib/operation-feedback";
import { fingerprintLabels, subscriptionLabels, templateKeys } from "../constants";
import {
  manualTemplateSchema,
  manualTemplateDefaults,
  manualTemplateConfig,
  type ManualTemplateValues,
} from "../lib/manual-template-schema";
import type { WorkbenchTemplate } from "../types";

const fields = [
  { name: "name", label: "模板名称", type: "text", hint: "例如：Plus 常用配置" },
  { name: "concurrency", label: "并发数", type: "number", hint: "0 表示不限流" },
  { name: "priority", label: "优先级", type: "number", hint: "请输入非负整数" },
  { name: "rate_multiplier", label: "计费倍率", type: "text", hint: "例如：1 或 0.5" },
  { name: "load_factor", label: "负载因子", type: "text", hint: "留空使用默认值" },
  { name: "proxy_id", label: "代理 ID", type: "text", hint: "目标站点代理 ID，留空直连" },
  { name: "group_ids", label: "分组 ID", type: "text", hint: "目标站点分组 ID，多个用逗号分隔" },
] as const;

export function ManualTemplateEditor(props: {
  template?: WorkbenchTemplate;
  revision: number;
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const form = useForm<ManualTemplateValues>({
    resolver: zodResolver(manualTemplateSchema),
    defaultValues: manualTemplateDefaults(props.template),
  });
  const save = useMutation({
    mutationFn: (values: ManualTemplateValues) =>
      api.saveWorkbenchTemplate({
        id: props.template?.id,
        name: values.name,
        revision: props.revision,
        config: manualTemplateConfig(values, props.template?.config),
      }),
    onSuccess: (result) => {
      client.setQueryData(templateKeys.library, result);
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "手动模板保存失败"),
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) props.onClose();
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <form
          onSubmit={form.handleSubmit((values) => save.mutate(values))}
          className="flex min-h-0 flex-col overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>{props.template ? "编辑手动模板" : "手动创建模板"}</DialogTitle>
          </DialogHeader>
          <DialogBody className="grid gap-4">
            <p className="text-xs text-muted-foreground">
              填写导入账号时使用的配置，保存后可直接选择此模板。
            </p>
            <fieldset disabled={save.isPending} className="grid min-w-0 gap-3 sm:grid-cols-2">
              <legend className="sr-only">模板配置</legend>
              {fields.map((field) => (
                <div key={field.name} className="grid min-w-0 content-start gap-1.5">
                  <label htmlFor={`manual-${field.name}`} className="text-sm">
                    {field.label}
                  </label>
                  <Input
                    id={`manual-${field.name}`}
                    type={field.type}
                    min={field.type === "number" ? 0 : undefined}
                    placeholder={field.hint}
                    aria-invalid={!!form.formState.errors[field.name]}
                    {...form.register(field.name, { valueAsNumber: field.type === "number" })}
                  />
                  {field.name === "concurrency" && (
                    <p className="text-xs text-muted-foreground">0 表示不限流</p>
                  )}
                  {form.formState.errors[field.name] && (
                    <p role="alert" className="text-xs text-destructive">
                      {form.formState.errors[field.name]?.message}
                    </p>
                  )}
                </div>
              ))}
              <div className="grid gap-1.5">
                <label htmlFor="manual-plan" className="text-sm">
                  订阅档位
                </label>
                <Controller
                  name="plan"
                  control={form.control}
                  render={({ field }) => (
                    <Select
                      disabled={save.isPending}
                      value={field.value || "auto"}
                      onValueChange={(value) => field.onChange(value === "auto" ? "" : value)}
                    >
                      <SelectTrigger id="manual-plan">
                        <SelectValue>{subscriptionLabels[field.value] || "自动识别"}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="auto">自动识别</SelectItem>
                        {Object.entries(subscriptionLabels).map(([value, label]) => (
                          <SelectItem key={value} value={value}>
                            {label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>
              <div className="grid gap-1.5">
                <label htmlFor="manual-fingerprint" className="text-sm">
                  Codex 指纹
                </label>
                <Controller
                  name="fingerprint"
                  control={form.control}
                  render={({ field }) => (
                    <Select
                      disabled={save.isPending}
                      value={field.value}
                      onValueChange={field.onChange}
                    >
                      <SelectTrigger id="manual-fingerprint">
                        <SelectValue>{fingerprintLabels[field.value]}</SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        {Object.entries(fingerprintLabels).map(([value, label]) => (
                          <SelectItem key={value} value={value}>
                            {label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>
              <label className="flex items-center gap-2 text-sm">
                <Controller
                  name="auto_pause_on_expired"
                  control={form.control}
                  render={({ field }) => (
                    <Checkbox
                      disabled={save.isPending}
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  )}
                />
                到期自动暂停
              </label>
            </fieldset>
            <div className="grid gap-2">
              <label className="text-sm">模型映射</label>
              <JsonEditorField
                control={form.control}
                name="model_mapping"
                aria-label="模型映射"
                readOnly={save.isPending}
                className="h-40"
              />
              <p className="text-xs text-muted-foreground">
                按“请求模型名: 实际模型名”填写，空对象表示不限制。
              </p>
              {form.formState.errors.model_mapping && (
                <p role="alert" className="text-xs text-destructive">
                  {form.formState.errors.model_mapping.message}
                </p>
              )}
            </div>
            <div className="grid gap-2">
              <label htmlFor="manual-notes" className="text-sm">
                备注
              </label>
              <Textarea id="manual-notes" disabled={save.isPending} {...form.register("notes")} />
              {form.formState.errors.notes && (
                <p role="alert" className="text-xs text-destructive">
                  {form.formState.errors.notes.message}
                </p>
              )}
            </div>
          </DialogBody>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={save.isPending}
              onClick={props.onClose}
            >
              取消
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? "正在保存" : "保存模板"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
