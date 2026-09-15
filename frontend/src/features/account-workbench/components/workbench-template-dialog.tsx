import { useEffect, useId, useState, type ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api, type WorkbenchTemplate, type WorkbenchTemplateSource } from "@/api";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { defaultConfig, workbenchKeys } from "../constants";
import { templateNameSchema, type TemplateNameValues } from "../lib/schemas";
import { WorkbenchTemplateSourcePicker } from "./workbench-template-source";
import { WorkbenchTemplateSummary } from "./workbench-template-summary";

export function WorkbenchTemplateDialog(props: {
  item?: WorkbenchTemplate;
  refreshSource?: boolean;
  onClose: () => void;
  onSaved?: (template: WorkbenchTemplate) => void;
}): ReactElement {
  const client = useQueryClient();
  const formID = useId();
  const sourcing = !props.item || !!props.refreshSource;
  const [source, setSource] = useState<WorkbenchTemplateSource | null>(null);
  const [sourceBlocked, setSourceBlocked] = useState(sourcing);
  const form = useForm<TemplateNameValues>({
    resolver: zodResolver(templateNameSchema),
    defaultValues: { name: props.item?.name ?? "" },
  });
  const config = source?.config ?? props.item?.config;
  useEffect(() => {
    if (source && !props.item && !form.getFieldState("name").isDirty)
      form.setValue("name", `${source.account_name} 配置`);
  }, [source, props.item, form]);
  const save = useMutation({
    mutationFn: (values: TemplateNameValues) => {
      if (sourcing && (!source || sourceBlocked)) throw new Error("请先读取来源账号配置");
      return api.saveWorkbenchTemplate(props.item?.id, {
        name: values.name,
        preferred: sourcing ? true : props.item?.preferred,
        revision: props.item?.revision,
        priority: source?.priority ?? props.item?.priority ?? 0,
        match: source?.match ?? props.item?.match ?? { plan_type: "", email_domain: "" },
        config: { ...defaultConfig, ...config },
        source_account_id: source?.account_id,
        source_revision: source?.source_revision,
      });
    },
    onSuccess: (value) => {
      toast.success(sourcing ? "配置模板已保存并使用" : "模板名称已更新");
      void client.invalidateQueries({ queryKey: workbenchKeys.templates });
      props.onSaved?.(value);
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "配置模板保存失败，请重新读取后重试"),
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{sourcing ? "读取线上账号配置" : "修改模板名称"}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <form
            id={formID}
            className="min-w-0 space-y-4"
            onSubmit={(event) => {
              event.stopPropagation();
              void form.handleSubmit((values) => save.mutate(values))(event);
            }}
          >
            {sourcing && (
              <WorkbenchTemplateSourcePicker
                item={props.item}
                disabled={save.isPending}
                onBlockedChange={setSourceBlocked}
                onApply={setSource}
              />
            )}
            <FormField
              label="模板名称"
              htmlFor={formID + "-name"}
              error={form.formState.errors.name?.message}
            >
              <Input
                id={formID + "-name"}
                disabled={save.isPending}
                aria-invalid={!!form.formState.errors.name}
                {...form.register("name")}
                placeholder={source ? `${source.account_name} 配置` : "模板名称"}
              />
            </FormField>
            {config && (
              <WorkbenchTemplateSummary
                config={config}
                match={source?.match ?? props.item?.match}
              />
            )}
          </form>
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" disabled={save.isPending} onClick={props.onClose}>
            取消
          </Button>
          <Button
            type="submit"
            form={formID}
            disabled={save.isPending || (sourcing && (sourceBlocked || !source))}
          >
            {sourcing ? "保存并使用" : "保存名称"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
