import { useId, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api, type WorkbenchTemplate, type WorkbenchTemplateSource } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { defaultConfig, workbenchKeys } from "../constants";
import { templateSchema, type TemplateValues } from "../lib/schemas";
import { WorkbenchGroupPicker } from "./group-picker";
import { WorkbenchTemplateSourcePicker } from "./workbench-template-source";

export function WorkbenchTemplateDialog(props: {
  item?: WorkbenchTemplate;
  refreshSource?: boolean;
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const formID = useId();
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
  const [source, setSource] = useState<WorkbenchTemplateSource | null>(null);
  const [sourceBlocked, setSourceBlocked] = useState(false);
  const form = useForm<TemplateValues>({
    resolver: zodResolver(templateSchema),
    defaultValues: {
      name: props.item?.name ?? "",
      preferred: props.item?.preferred ?? false,
      priority: props.item?.priority ?? 0,
      match: props.item?.match ?? { plan_type: "", email_domain: "" },
      config: { ...defaultConfig, ...props.item?.config },
    },
  });
  const save = useMutation({
    mutationFn: (values: TemplateValues) =>
      api.saveWorkbenchTemplate(props.item?.id, {
        ...values,
        revision: props.item?.revision,
        source_account_id: source?.account_id,
        source_revision: source?.source_revision,
      }),
    onSuccess: () => {
      toast.success("配置模板已保存");
      void client.invalidateQueries({ queryKey: workbenchKeys.templates });
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "配置模板保存失败，请检查输入或重新读取模板"),
  });
  const pending = save.isPending || sourceBlocked;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) props.onClose();
      }}
    >
      <DialogContent width="wide">
        <DialogHeader>
          <DialogTitle>{props.item ? "编辑账号配置模板" : "新增账号配置模板"}</DialogTitle>
          <DialogDescription>
            设置导入账号的分组、倍率与调度参数，也可按账号 ID 提取已有配置。
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <form
            id={formID}
            className="min-w-0 space-y-4"
            onSubmit={form.handleSubmit((value) => save.mutate(value))}
          >
            <WorkbenchTemplateSourcePicker
              item={props.item}
              value={source}
              disabled={save.isPending}
              refresh={props.refreshSource ?? false}
              current={form.getValues}
              onBlockedChange={setSourceBlocked}
              onApply={(value) => {
                setSource(value);
                form.setValue(
                  "config",
                  { ...defaultConfig, ...value.config },
                  { shouldValidate: true, shouldDirty: true },
                );
                form.setValue("match", value.match, { shouldValidate: true, shouldDirty: true });
                form.setValue("priority", value.priority, {
                  shouldValidate: true,
                  shouldDirty: true,
                });
                form.setValue("preferred", true, { shouldDirty: true });
                if (!form.getValues("name")) form.setValue("name", `${value.account_name} 配置`);
              }}
            />
            <div className="grid gap-3 sm:grid-cols-2">
              <FormField
                label="模板名称"
                htmlFor="workbench-template-name"
                error={form.formState.errors.name?.message}
              >
                <Input
                  id="workbench-template-name"
                  disabled={pending}
                  aria-invalid={!!form.formState.errors.name}
                  {...form.register("name")}
                />
              </FormField>
              <FormField
                label="匹配优先级"
                htmlFor="workbench-template-priority"
                error={form.formState.errors.priority?.message}
              >
                <Input
                  id="workbench-template-priority"
                  type="number"
                  min={0}
                  disabled={pending}
                  {...form.register("priority", { valueAsNumber: true })}
                />
              </FormField>
              <FormField
                label="套餐条件"
                htmlFor="workbench-plan"
                error={form.formState.errors.match?.plan_type?.message}
              >
                <Input
                  id="workbench-plan"
                  placeholder="留空表示不限套餐"
                  disabled={pending}
                  {...form.register("match.plan_type")}
                />
              </FormField>
              <FormField
                label="邮箱域名条件"
                htmlFor="workbench-domain"
                error={form.formState.errors.match?.email_domain?.message}
              >
                <Input
                  id="workbench-domain"
                  placeholder="example.com，留空表示不限"
                  disabled={pending}
                  {...form.register("match.email_domain")}
                />
              </FormField>
              <FormField
                label="并发数"
                htmlFor="workbench-concurrency"
                error={form.formState.errors.config?.concurrency?.message}
              >
                <Input
                  id="workbench-concurrency"
                  type="number"
                  min={1}
                  disabled={pending}
                  {...form.register("config.concurrency", { valueAsNumber: true })}
                />
              </FormField>
              <FormField
                label="账号优先级"
                htmlFor="workbench-account-priority"
                error={form.formState.errors.config?.priority?.message}
              >
                <Input
                  id="workbench-account-priority"
                  type="number"
                  min={0}
                  disabled={pending}
                  {...form.register("config.priority", { valueAsNumber: true })}
                />
              </FormField>
              <FormField
                label="倍率"
                htmlFor="workbench-rate"
                error={form.formState.errors.config?.rate_multiplier?.message}
              >
                <Input
                  id="workbench-rate"
                  inputMode="decimal"
                  disabled={pending}
                  {...form.register("config.rate_multiplier")}
                />
              </FormField>
              <FormField
                label="备注"
                htmlFor="workbench-notes"
                error={form.formState.errors.config?.notes?.message}
              >
                <Input id="workbench-notes" disabled={pending} {...form.register("config.notes")} />
              </FormField>
            </div>
            {groups.isPending && <ContentLoading label="正在读取分组" compact />}
            {!groups.data && groups.isError && (
              <ContentRetry pending={groups.isFetching} onRetry={() => void groups.refetch()} />
            )}
            {groups.data && (
              <Controller
                control={form.control}
                name="config.group_ids"
                render={({ field }) => (
                  <WorkbenchGroupPicker
                    groups={groups.data}
                    value={field.value}
                    onChange={field.onChange}
                    disabled={pending}
                  />
                )}
              />
            )}
            <label className="flex items-center gap-2 text-sm">
              <Controller
                control={form.control}
                name="preferred"
                render={({ field }) => (
                  <Checkbox
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={pending}
                  />
                )}
              />
              设为首选模板
            </label>
            <label className="flex items-center gap-2 text-sm">
              <Controller
                control={form.control}
                name="config.auto_pause_on_expired"
                render={({ field }) => (
                  <Checkbox
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={pending}
                  />
                )}
              />
              到期自动暂停
            </label>
          </form>
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" disabled={save.isPending} onClick={props.onClose}>
            取消
          </Button>
          <Button type="submit" form={formID} disabled={pending || !groups.data}>
            {save.isPending ? "正在保存…" : "保存模板"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
