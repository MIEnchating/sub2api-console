import { useEffect, useId, useRef, type ReactElement } from "react";
import { useMutation } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api, type WorkbenchSMSReceipt } from "@/api";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
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
import { smsInspectSchema, type SMSInspectValues } from "../lib/sms-inspect-schema";

export function WorkbenchSMSInspect(props: {
  receipt: WorkbenchSMSReceipt;
  onClose: () => void;
}): ReactElement {
  const formID = useId();
  const payload = useRef<SMSInspectValues | null>(null);
  const form = useForm<SMSInspectValues>({
    resolver: zodResolver(smsInspectSchema),
    defaultValues: {
      provider: props.receipt.provider,
      api_key: "",
      service_id: "",
      custom_entries: "",
    },
  });
  const inspect = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      if (!payload.current) throw new Error("请重新填写供应商配置");
      const pending = api.inspectWorkbenchSMSReceipt(props.receipt.id, {
        ...payload.current,
        scope: props.receipt.scope,
      });
      payload.current = null;
      return pending;
    },
    onSuccess: (value) => {
      toast.success(value.message);
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "原订单核对失败，请检查供应商配置后重新填写"),
  });
  useEffect(
    () => () => {
      payload.current = null;
      form.reset();
    },
    [form],
  );
  return (
    <Dialog
      open
      onOpenChange={(value) => {
        if (!value && !inspect.isPending) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>核对原短信订单</DialogTitle>
          <DialogDescription>
            查询已购号码的收码状态。验证码请在供应商查看，随后人工完成原登录页面。
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <form
            id={formID}
            className="grid min-w-0 gap-3"
            onSubmit={form.handleSubmit((value) => {
              payload.current = value;
              form.reset();
              inspect.mutate();
            })}
          >
            <p className="text-sm wrap-anywhere">
              订单：{props.receipt.order_id} · {props.receipt.phone}
            </p>
            {props.receipt.provider !== "custom" && (
              <FormField
                label="原供应商 API Key"
                htmlFor={`${formID}-key`}
                error={form.formState.errors.api_key?.message}
              >
                <Input
                  id={`${formID}-key`}
                  type="password"
                  autoComplete="off"
                  disabled={inspect.isPending}
                  aria-invalid={!!form.formState.errors.api_key}
                  {...form.register("api_key")}
                />
              </FormField>
            )}
            {props.receipt.provider === "luban" && (
              <FormField
                label="原供应商编号"
                htmlFor={`${formID}-service`}
                error={form.formState.errors.service_id?.message}
              >
                <Input
                  id={`${formID}-service`}
                  disabled={inspect.isPending}
                  aria-invalid={!!form.formState.errors.service_id}
                  {...form.register("service_id")}
                />
              </FormField>
            )}
            {props.receipt.provider === "custom" && (
              <FormField
                label="原自定义接码列表"
                htmlFor={`${formID}-custom`}
                error={form.formState.errors.custom_entries?.message}
              >
                <Textarea
                  id={`${formID}-custom`}
                  className="h-32"
                  autoComplete="off"
                  disabled={inspect.isPending}
                  aria-invalid={!!form.formState.errors.custom_entries}
                  {...form.register("custom_entries")}
                />
              </FormField>
            )}
          </form>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" disabled={inspect.isPending} onClick={props.onClose}>
            返回
          </Button>
          <Button type="submit" form={formID} disabled={inspect.isPending}>
            {inspect.isPending ? "正在核对…" : "查询原订单"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
