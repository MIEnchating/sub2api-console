import { ContentRetry } from "@/components/content-retry";
import { FieldError } from "@/components/field-error";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";

import { api, ApiError, type AccountModelSyncSettings } from "@/api";
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
  const baseline = useRef<string[] | null>(null);
  const appliedSettings = useRef<AccountModelSyncSettings | null>(null);
  const submitting = useRef(false);
  const mounted = useRef(false);
  const [conflict, setConflict] = useState(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const settings = useQuery({
    queryKey: ["account-model-sync-settings"],
    queryFn: ({ signal }) => api.accountModelSyncSettings(signal),
  });
  const form = useForm<ModelSyncSettingsValues>({
    resolver: zodResolver(modelSyncSettingsSchema),
    defaultValues: { blockedPatterns: "" },
  });
  const save = useMutation({
    retry: false,
    networkMode: "always",
    mutationFn: (values: AccountModelSyncSettings & { expected_patterns: string[] }) =>
      api.updateAccountModelSyncSettings(values),
    onSuccess: async (value) => {
      await queryClient.cancelQueries({ queryKey: ["account-model-sync-settings"], exact: true });
      queryClient.setQueryData(["account-model-sync-settings"], value);
      void queryClient.invalidateQueries({
        queryKey: ["account-model-sync-settings"],
        exact: true,
      });
      void queryClient.invalidateQueries({ queryKey: ["account-model-sync-preview"] });
      if (!mounted.current) return;
      appliedSettings.current =
        queryClient.getQueryData<AccountModelSyncSettings>(["account-model-sync-settings"]) ?? null;
      baseline.current = [...value.blocked_patterns];
      form.reset({ blockedPatterns: value.blocked_patterns.join("\n") });
      toast.success("全局屏蔽模型已保存");
    },
    onError: (error) => {
      if (!mounted.current) return;
      if (error instanceof ApiError && error.code === "model_sync_settings_changed") {
        setConflict(true);
        void queryClient.invalidateQueries({
          queryKey: ["account-model-sync-settings"],
          exact: true,
        });
      }
      notifyOperationError(error, "全局屏蔽模型保存失败");
    },
    onSettled: () => {
      submitting.current = false;
    },
  });
  const reload = useMutation({
    retry: false,
    networkMode: "always",
    mutationFn: () => settings.refetch({ throwOnError: true }),
    onSuccess: (result) => {
      if (!mounted.current || !result.data) return;
      baseline.current = [...result.data.blocked_patterns];
      form.reset({ blockedPatterns: result.data.blocked_patterns.join("\n") });
      setConflict(false);
    },
    onError: (error) => {
      if (mounted.current) notifyOperationError(error, "屏蔽规则读取失败，请重试");
    },
  });

  useEffect(() => {
    if (
      !settings.data ||
      settings.error ||
      settings.data === appliedSettings.current ||
      form.formState.isDirty ||
      save.isPending ||
      conflict
    )
      return;
    appliedSettings.current = settings.data;
    baseline.current = [...settings.data.blocked_patterns];
    form.reset({ blockedPatterns: settings.data.blocked_patterns.join("\n") });
  }, [settings.data, settings.error, form, form.formState.isDirty, save.isPending, conflict]);

  if (settings.isLoading && settings.data === undefined) {
    return (
      <Card
        size="sm"
        className="h-full min-h-0"
        role="status"
        aria-busy="true"
        aria-label="正在读取全局屏蔽模型"
      >
        <CardHeader>
          <CardTitle>全局屏蔽模型</CardTitle>
          <Skeleton className="h-4 w-3/4" />
        </CardHeader>
        <CardContent className="flex min-h-0 flex-1 flex-col gap-3">
          <Skeleton className="h-4 w-24 shrink-0" />
          <div
            aria-hidden="true"
            className="flex min-h-32 flex-1 flex-col gap-2 rounded-lg border p-3"
          >
            <Skeleton className="h-4 w-1/2" />
            <Skeleton className="h-4 w-1/3" />
            <Skeleton className="h-4 w-2/5" />
          </div>
        </CardContent>
        <SettingsFooter>
          <Skeleton className="h-8 w-28" />
        </SettingsFooter>
      </Card>
    );
  }

  if (!settings.data) {
    return <ContentRetry pending={settings.isFetching} onRetry={() => void settings.refetch()} />;
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
          onSubmit={form.handleSubmit((values) => {
            if (submitting.current || reload.isPending || conflict || baseline.current === null)
              return;
            submitting.current = true;
            save.mutate({
              blocked_patterns: parseModelBlockPatterns(values.blockedPatterns),
              expected_patterns: [...baseline.current],
            });
          })}
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
                disabled={save.isPending || reload.isPending}
                aria-invalid={error ? "true" : undefined}
                aria-describedby={error ? "model-sync-blocked-patterns-error" : undefined}
                placeholder={"例如：\nclaude-*\ngemini-*\n*-image-*"}
              />
            </label>
            <FieldError id="model-sync-blocked-patterns-error" message={error} />
            {settings.error ? (
              <ContentRetry pending={settings.isFetching} onRetry={() => void settings.refetch()} />
            ) : null}
          </div>
          <SettingsFooter>
            {conflict ? (
              <Button type="button" disabled={reload.isPending} onClick={() => reload.mutate()}>
                {reload.isPending ? "正在加载最新规则…" : "加载最新规则并替换草稿"}
              </Button>
            ) : null}
            <Button
              type="submit"
              disabled={
                save.isPending ||
                reload.isPending ||
                conflict ||
                Boolean(settings.error) ||
                !form.formState.isDirty
              }
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
