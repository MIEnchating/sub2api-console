import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";

import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { notifyOperationError } from "@/lib/operation-feedback";
import { SettingsFooter } from "./settings-footer";
import {
  modelSyncSettingsSchema,
  parseModelBlockPatterns,
  type ModelSyncSettingsValues,
} from "../lib/model-sync-settings-schema";

export function ModelSyncSettingsCard() {
  const queryClient = useQueryClient();
  const settings = useQuery({
    queryKey: ["account-model-sync-settings"],
    queryFn: api.accountModelSyncSettings,
  });
  const form = useForm<ModelSyncSettingsValues>({
    resolver: zodResolver(modelSyncSettingsSchema),
    defaultValues: { blockedPatterns: "" },
  });
  const save = useMutation({
    mutationFn: (values: ModelSyncSettingsValues) =>
      api.updateAccountModelSyncSettings({
        blocked_patterns: parseModelBlockPatterns(values.blockedPatterns),
      }),
    onSuccess: (value) => {
      queryClient.setQueryData(["account-model-sync-settings"], value);
      void queryClient.invalidateQueries({ queryKey: ["account-model-sync-preview"] });
      form.reset({ blockedPatterns: value.blocked_patterns.join("\n") });
      toast.success("全局屏蔽模型已保存");
    },
    onError: (error) => notifyOperationError(error, "全局屏蔽模型保存失败"),
  });

  useEffect(() => {
    if (!settings.data || form.formState.isDirty) return;
    form.reset({ blockedPatterns: settings.data.blocked_patterns.join("\n") });
  }, [settings.data, form, form.formState.isDirty]);

  if (settings.isLoading && settings.data === undefined) {
    return (
      <Card size="sm" className="h-full" aria-label="正在读取全局屏蔽模型">
        <CardHeader>
          <CardTitle>全局屏蔽模型</CardTitle>
        </CardHeader>
        <CardContent>
          <Skeleton className="h-36" />
        </CardContent>
      </Card>
    );
  }

  const field = form.register("blockedPatterns");
  const error = form.formState.errors.blockedPatterns?.message;
  return (
    <Card size="sm" className="h-full min-h-0 min-w-0" data-testid="model-sync-settings-card">
      <CardHeader className="shrink-0">
        <CardTitle>全局屏蔽模型</CardTitle>
        <CardDescription>
          每行一条规则，支持 * 匹配任意字符、? 匹配单个字符；同步账号模型时自动屏蔽。
        </CardDescription>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-1 flex-col group-data-[size=sm]/card:p-0">
        <form
          className="flex min-h-0 flex-1 flex-col"
          onSubmit={form.handleSubmit((values) => save.mutate(values))}
        >
          <div
            data-slot="settings-scroll"
            className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto overscroll-contain px-3 py-3"
          >
            <label
              className="flex min-h-32 flex-1 flex-col gap-1.5"
              htmlFor="model-sync-blocked-patterns"
            >
              <span className="text-sm font-medium">屏蔽规则</span>
              <Textarea
                {...field}
                id="model-sync-blocked-patterns"
                className="field-sizing-fixed min-h-0 max-h-none flex-1 resize-none font-mono text-xs leading-5"
                rows={7}
                spellCheck={false}
                aria-invalid={error ? "true" : undefined}
                aria-describedby={error ? "model-sync-blocked-patterns-error" : undefined}
                placeholder={"例如：\nclaude-*\ngemini-*\n*-image-*"}
              />
            </label>
            {error ? (
              <p className="text-destructive text-sm" id="model-sync-blocked-patterns-error">
                {error}
              </p>
            ) : null}
            {settings.error ? (
              <p className="text-destructive text-sm" role="alert">
                全局屏蔽模型设置读取失败，请刷新后重试
              </p>
            ) : null}
          </div>
          <SettingsFooter>
            <Button
              type="submit"
              disabled={save.isPending || Boolean(settings.error) || !form.formState.isDirty}
            >
              <Save aria-hidden="true" />
              {save.isPending ? "保存中…" : "保存屏蔽规则"}
            </Button>
          </SettingsFooter>
        </form>
      </CardContent>
    </Card>
  );
}
