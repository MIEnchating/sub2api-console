import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { Plus } from "lucide-react";
import { FieldError } from "@/components/field-error";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  manualSyncModelSchema,
  type ManualSyncModelFormValues,
} from "../lib/manual-sync-model-schema";
import { modelMatchesBlockPatterns } from "../lib/model-block-patterns";

export function ManualSyncModelForm(props: {
  disabled: boolean;
  blockedPatterns: readonly string[];
  onAdd: (model: string) => void;
}) {
  const form = useForm<ManualSyncModelFormValues>({
    resolver: zodResolver(manualSyncModelSchema),
    defaultValues: { model: "" },
  });
  return (
    <form
      className="grid gap-1 sm:max-w-2xl"
      onSubmit={form.handleSubmit((value) => {
        if (modelMatchesBlockPatterns(value.model, props.blockedPatterns)) {
          form.setError("model", {
            message: "该模型已被全局屏蔽，请先在全局屏蔽模型设置中移除对应规则",
          });
          return;
        }
        props.onAdd(value.model);
        form.reset();
      })}
    >
      <div className="flex min-w-0 gap-2">
        <Input
          {...form.register("model")}
          aria-label="手动输入同步模型"
          placeholder="手动输入同步模型"
          disabled={props.disabled}
          aria-invalid={Boolean(form.formState.errors.model)}
        />
        <Button type="submit" variant="outline" disabled={props.disabled}>
          <Plus aria-hidden="true" />
          添加模型
        </Button>
      </div>
      <FieldError message={form.formState.errors.model?.message} />
    </form>
  );
}
