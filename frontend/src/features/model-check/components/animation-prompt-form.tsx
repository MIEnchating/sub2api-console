import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState, type ReactElement } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { toast } from "sonner";
import { api, type AnimationPromptSettings } from "@/api";
import { FieldError } from "@/components/field-error";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { DialogBody, DialogFooter } from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { animationPromptSchema, type AnimationPromptForm } from "../lib/animation-prompt-schema";
import { animationPromptQueryKey } from "../lib/animation-prompt-query";

export function AnimationPromptFormEditor(props: {
  value: AnimationPromptSettings;
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const formId = useId();
  const [version] = useState(props.value.version);
  const form = useForm<AnimationPromptForm>({
    resolver: zodResolver(animationPromptSchema),
    defaultValues: { templates: props.value.templates, default_id: props.value.default_id },
  });
  const entries = useFieldArray({ control: form.control, name: "templates", keyName: "fieldKey" });
  const defaultId = form.watch("default_id");
  const save = useMutation({
    mutationFn: (value: AnimationPromptForm) => api.saveAnimationPrompt({ ...value, version }),
    onSuccess: (value) => {
      client.setQueryData(animationPromptQueryKey, value);
      toast.success("动画提示词已保存");
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "动画提示词保存失败"),
  });
  return (
    <form
      onSubmit={(event) => {
        event.stopPropagation();
        void form.handleSubmit((value) => save.mutate(value))(event);
      }}
      noValidate
    >
      <DialogBody className="space-y-4">
        {entries.fields.map((item, index) => {
          const prefix = `${formId}-${item.id}`;
          const errors = form.formState.errors.templates?.[index];
          return (
            <fieldset
              key={item.fieldKey}
              className="min-w-0 space-y-2 rounded-lg border p-3"
              disabled={save.isPending}
            >
              <legend className="px-1 text-sm font-medium">提示词 {index + 1}</legend>
              <label htmlFor={`${prefix}-name`} className="text-sm font-medium">
                名称
              </label>
              <Input
                id={`${prefix}-name`}
                {...form.register(`templates.${index}.name`)}
                maxLength={80}
                aria-invalid={!!errors?.name}
                aria-describedby={errors?.name ? `${prefix}-name-error` : undefined}
              />
              <FieldError id={`${prefix}-name-error`} message={errors?.name?.message} />
              <label htmlFor={`${prefix}-prompt`} className="text-sm font-medium">
                动画提示词
              </label>
              <Textarea
                id={`${prefix}-prompt`}
                {...form.register(`templates.${index}.prompt`)}
                maxLength={10000}
                rows={6}
                aria-invalid={!!errors?.prompt}
                aria-describedby={errors?.prompt ? `${prefix}-prompt-error` : undefined}
              />
              <FieldError id={`${prefix}-prompt-error`} message={errors?.prompt?.message} />
              <div className="flex flex-wrap items-center justify-between gap-2">
                <label className="flex items-center gap-2 text-sm">
                  <input type="radio" value={item.id} {...form.register("default_id")} />
                  设为默认
                </label>
                <Button
                  type="button"
                  variant="outline"
                  disabled={save.isPending || entries.fields.length === 1 || defaultId === item.id}
                  onClick={() => entries.remove(index)}
                >
                  删除提示词
                </Button>
              </div>
              {defaultId === item.id ? (
                <p className="text-xs text-muted-foreground">
                  当前默认提示词；删除前请先将其他提示词设为默认。
                </p>
              ) : null}
            </fieldset>
          );
        })}
        <FieldError message={form.formState.errors.default_id?.message} />
        <Button
          type="button"
          variant="outline"
          disabled={save.isPending || entries.fields.length >= 32}
          onClick={() =>
            entries.append({
              id: crypto.getRandomValues(new Uint32Array(4)).join("-"),
              name: "",
              prompt: "",
            })
          }
        >
          新增提示词
        </Button>
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
