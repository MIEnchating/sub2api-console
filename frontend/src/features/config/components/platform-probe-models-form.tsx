import { zodResolver } from "@hookform/resolvers/zod";
import { FieldError } from "@/components/field-error";
import { useQuery } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { groupPlatformOptions } from "@/features/accounts/lib/account-labels";
import { api } from "@/api";
import { orderedDictionaryOptions } from "@/lib/domain-dictionaries";
import { SettingsFooter } from "./settings-footer";
import {
  platformProbeModelsSchema,
  type PlatformProbeModelsValues,
} from "../lib/platform-probe-models-schema";

const emptyPlatformProbeModels: PlatformProbeModelsValues = {
  openai: "",
  anthropic: "",
  gemini: "",
  antigravity: "",
  grok: "",
  kimi: "",
  zhipu: "",
  deepseek: "",
  composite: "",
};

function isPlatformProbeKey(value: string): value is keyof PlatformProbeModelsValues {
  return Object.hasOwn(emptyPlatformProbeModels, value);
}

function platformProbeModelFormValues(models: Record<string, string>): PlatformProbeModelsValues {
  return {
    ...emptyPlatformProbeModels,
    ...Object.fromEntries(
      Object.entries(models).filter(([platform]) => isPlatformProbeKey(platform)),
    ),
  } as PlatformProbeModelsValues;
}

function platformProbeModelsFromForm(
  values: PlatformProbeModelsValues,
  current: Record<string, string>,
): Record<string, string> {
  const models = { ...current };
  for (const [platform, value] of Object.entries(values)) {
    const model = value.trim();
    if (model) models[platform] = model;
    else delete models[platform];
  }
  return models;
}

export function PlatformProbeModelsForm(props: {
  models: Record<string, string>;
  pending: boolean;
  disabled?: boolean;
  onSubmit: (models: Record<string, string>) => void | Promise<void>;
}) {
  const platformDictionary = useQuery({
    queryKey: ["dictionaries", "platform"],
    queryFn: () => api.dictionaries("platform"),
  });
  const form = useForm<PlatformProbeModelsValues>({
    resolver: zodResolver(platformProbeModelsSchema),
    defaultValues: platformProbeModelFormValues(props.models),
  });
  const isDirty = form.formState.isDirty;

  useEffect(() => {
    if (isDirty) return;
    form.reset(platformProbeModelFormValues(props.models));
  }, [form, isDirty, props.models]);
  const platformOptions = orderedDictionaryOptions(
    platformDictionary.data?.items,
    groupPlatformOptions,
  ).flatMap((option) =>
    isPlatformProbeKey(option.value) ? [{ ...option, value: option.value }] : [],
  );

  return (
    <form
      className="flex h-full min-h-0 flex-col overflow-hidden"
      onSubmit={form.handleSubmit(async (values) => {
        try {
          await props.onSubmit(platformProbeModelsFromForm(values, props.models));
          form.reset(values);
        } catch {
          // 父级 mutation 展示写入错误，表单保留草稿供重试。
        }
      })}
    >
      <div
        data-slot="settings-scroll"
        className="grid min-h-0 flex-1 content-start gap-x-4 gap-y-3 overflow-y-auto overscroll-contain px-3 py-3 sm:grid-cols-2 lg:grid-cols-3"
      >
        {platformOptions.map((platform) => {
          const error = form.formState.errors[platform.value]?.message;
          return (
            <label className="grid min-w-0 gap-1.5 text-sm" key={platform.value}>
              <span className="font-medium">{platform.label}</span>
              <Input
                aria-label={`${platform.label} 默认探活模型`}
                aria-invalid={Boolean(error)}
                disabled={props.disabled || props.pending}
                placeholder="留空则自动选择"
                {...form.register(platform.value)}
              />
              <FieldError message={error} />
            </label>
          );
        })}
      </div>
      <SettingsFooter>
        <Button type="submit" disabled={props.pending || props.disabled || !form.formState.isDirty}>
          <Save aria-hidden="true" />
          {props.pending ? "保存中…" : "保存默认探活模型"}
        </Button>
      </SettingsFooter>
    </form>
  );
}
