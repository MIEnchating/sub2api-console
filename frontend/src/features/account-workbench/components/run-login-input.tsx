import { useId, type ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { loginInputSchema, type LoginInputValues } from "../lib/login-input-schema";
import { loginInputLabels, runKeys } from "../constants";
import type { WorkbenchLoginPrompt } from "../types";

export function RunLoginInput(props: {
  runID: string;
  itemID: string;
  email: string;
  prompt: WorkbenchLoginPrompt;
  onClose: () => void;
}): ReactElement {
  const id = useId();
  const client = useQueryClient();
  const form = useForm<LoginInputValues>({
    resolver: zodResolver(loginInputSchema),
    defaultValues: { kind: props.prompt.kind, value: "" },
  });
  const submit = useMutation({
    gcTime: 0,
    mutationFn: (value: { value: string; action?: "resend_email" }) =>
      api.workbenchLoginInput(props.runID, props.itemID, {
        prompt_id: props.prompt.id,
        value: value.value,
        ...(value.action ? { action: value.action } : {}),
      }),
    onSuccess: () => {
      form.reset();
      void client.invalidateQueries({ queryKey: runKeys.list });
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "验证内容提交失败"),
  });
  const password = props.prompt.kind === "password";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>继续授权 · {props.email}</DialogTitle>
        </DialogHeader>
        <form onSubmit={form.handleSubmit((value) => submit.mutate(value))}>
          <DialogBody>
            <label htmlFor={id} className="text-sm font-medium">
              {loginInputLabels[props.prompt.kind]}
            </label>
            <Input
              id={id}
              type={password ? "password" : "text"}
              inputMode={password ? "text" : "numeric"}
              autoComplete={password ? "off" : "one-time-code"}
              maxLength={password ? 4096 : 6}
              disabled={submit.isPending}
              aria-invalid={Boolean(form.formState.errors.value)}
              aria-describedby={form.formState.errors.value ? `${id}-error` : undefined}
              {...form.register("value")}
            />
            {form.formState.errors.value && (
              <p id={`${id}-error`} className="mt-1 text-sm text-destructive">
                {form.formState.errors.value.message}
              </p>
            )}
          </DialogBody>
          <DialogFooter>
            {props.prompt.kind === "email_code" && (
              <Button
                type="button"
                variant="outline"
                disabled={submit.isPending}
                onClick={() => submit.mutate({ value: "", action: "resend_email" })}
              >
                重发邮箱验证码
              </Button>
            )}
            <Button type="button" variant="outline" onClick={props.onClose}>
              关闭
            </Button>
            <Button type="submit" disabled={submit.isPending}>
              {submit.isPending ? "正在提交…" : "提交验证"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
