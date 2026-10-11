import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PencilLine } from "lucide-react";
import { useId, useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { FieldError } from "@/components/field-error";
import { Button } from "@/components/ui/button";
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
import { animationPromptSchema, type AnimationPromptForm } from "../lib/animation-prompt-schema";

const queryKey = ["model-animation", "prompt"] as const;

export function AnimationPromptDialog(props: { disabled?: boolean }): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        type="button"
        variant="outline"
        disabled={props.disabled}
        onClick={() => setOpen(true)}
        aria-label="编辑动画提示词"
      >
        <PencilLine aria-hidden="true" />
        提示词设置
      </Button>
      {open ? <PromptEditor onClose={() => setOpen(false)} /> : null}
    </>
  );
}

function PromptEditor(props: { onClose: () => void }): ReactElement {
  const settings = useQuery({
    queryKey,
    queryFn: ({ signal }) => api.animationPrompt(signal),
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent width="wide">
        <DialogHeader>
          <DialogTitle>动画提示词设置</DialogTitle>
          <DialogDescription>
            所有账号、自定义接口和自动检测共用一套提示词。保存后新建的动画任务立即使用，已开始的任务和历史记录保持原提示词。
          </DialogDescription>
        </DialogHeader>
        {settings.data ? (
          <PromptForm value={settings.data} onClose={props.onClose} />
        ) : (
          <>
            <DialogBody>
              {settings.isPending ? (
                <ContentLoading label="正在读取动画提示词" />
              ) : (
                <ContentRetry
                  onRetry={() => void settings.refetch()}
                  pending={settings.isFetching}
                />
              )}
            </DialogBody>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={props.onClose}>
                取消
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

function PromptForm(props: {
  value: { prompt: string; version: number };
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const promptID = useId();
  const [version] = useState(props.value.version);
  const form = useForm<AnimationPromptForm>({
    resolver: zodResolver(animationPromptSchema),
    defaultValues: { prompt: props.value.prompt },
  });
  const save = useMutation({
    mutationFn: (value: AnimationPromptForm) => api.saveAnimationPrompt({ ...value, version }),
    onSuccess: (value) => {
      client.setQueryData(queryKey, value);
      toast.success("动画提示词已保存");
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "动画提示词保存失败"),
  });
  const error = form.formState.errors.prompt;
  return (
    <form
      onSubmit={(event) => {
        event.stopPropagation();
        void form.handleSubmit((value) => save.mutate(value))(event);
      }}
      noValidate
    >
      <DialogBody className="space-y-2">
        <label htmlFor={promptID} className="text-sm font-medium">
          提示词
        </label>
        <Textarea
          id={promptID}
          aria-label="动画提示词"
          {...form.register("prompt")}
          disabled={save.isPending}
          aria-invalid={!!error}
          aria-describedby={error ? `${promptID}-error` : undefined}
          maxLength={10000}
          rows={8}
        />
        <FieldError id={`${promptID}-error`} message={error?.message} />
        <p className="text-right text-xs text-muted-foreground">
          {form.watch("prompt").length}/10000
        </p>
      </DialogBody>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={props.onClose}>
          取消
        </Button>
        <Button type="submit" disabled={save.isPending} aria-busy={save.isPending}>
          {save.isPending ? "正在保存" : "保存提示词"}
        </Button>
      </DialogFooter>
    </form>
  );
}
