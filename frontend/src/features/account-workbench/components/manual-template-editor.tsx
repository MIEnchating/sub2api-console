import type { ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Save } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { JsonEditorField } from "@/components/json-editor/form-field";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { templateKeys } from "../constants";
import {
  manualTemplateSchema,
  manualTemplateDefaults,
  manualTemplateConfig,
  type ManualTemplateValues,
} from "../lib/manual-template-schema";
import type { WorkbenchTemplate } from "../types";
import { TemplateSettingsFields } from "./template-settings-fields";

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
    onError: (error) => notifyOperationError(error, "模板保存失败"),
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) props.onClose();
      }}
    >
      <DialogContent
        width="wide"
        render={<form onSubmit={form.handleSubmit((values) => save.mutate(values))} />}
      >
        <DialogHeader>
          <DialogTitle>{props.template ? "编辑配置模板" : "手动创建模板"}</DialogTitle>
          <DialogDescription>
            设置导入账号时使用的配置，保存模板不会修改线上账号。
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid content-start gap-5 md:grid-cols-2">
          <div className="grid min-w-0 content-start gap-4">
            <section
              aria-label="基本信息"
              className="grid min-w-0 gap-2 rounded-lg border bg-muted/20 p-4"
            >
              <label htmlFor="manual-name" className="text-sm font-medium">
                模板名称
              </label>
              <Input
                id="manual-name"
                placeholder="例如：Plus 常用配置"
                disabled={save.isPending}
                aria-invalid={!!form.formState.errors.name}
                {...form.register("name")}
              />
              {form.formState.errors.name && (
                <p role="alert" className="text-xs text-destructive">
                  {form.formState.errors.name.message}
                </p>
              )}
              {props.template?.source_id && (
                <p className="text-xs text-muted-foreground wrap-anywhere">
                  来源：{props.template.source_name || `账号 #${props.template.source_id}`}
                  。重新同步会覆盖手动修改的配置。
                </p>
              )}
            </section>
            <TemplateSettingsFields form={form} pending={save.isPending} />
          </div>
          <div className="grid min-w-0 content-start gap-4">
            <section aria-label="模型映射设置" className="grid min-w-0 gap-3 rounded-lg border p-4">
              <h3 className="text-sm font-medium">模型映射</h3>
              <p className="text-xs text-muted-foreground">
                按“请求模型名: 实际模型名”填写，空对象表示不限制。
              </p>
              <JsonEditorField
                control={form.control}
                name="model_mapping"
                aria-label="模型映射"
                readOnly={save.isPending}
                className="h-56 sm:h-64"
              />
              {form.formState.errors.model_mapping && (
                <p role="alert" className="text-xs text-destructive">
                  {form.formState.errors.model_mapping.message}
                </p>
              )}
            </section>
            <section aria-label="模板备注" className="grid min-w-0 gap-2 rounded-lg border p-4">
              <label htmlFor="manual-notes" className="text-sm font-medium">
                备注
              </label>
              <Textarea
                id="manual-notes"
                className="min-h-24 resize-y"
                placeholder="记录模板用途或使用说明（选填）"
                disabled={save.isPending}
                {...form.register("notes")}
              />
              {form.formState.errors.notes && (
                <p role="alert" className="text-xs text-destructive">
                  {form.formState.errors.notes.message}
                </p>
              )}
            </section>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" disabled={save.isPending} onClick={props.onClose}>
            取消
          </Button>
          <Button type="submit" disabled={save.isPending}>
            <Save aria-hidden="true" />
            {save.isPending ? "正在保存" : "保存模板"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
