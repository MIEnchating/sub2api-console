import { zodResolver } from "@hookform/resolvers/zod";
import { Save } from "lucide-react";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { groupPlatformOptions } from "@/features/accounts/lib/account-labels";
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

function platformProbeModelFormValues(models: Record<string, string>): PlatformProbeModelsValues {
  return {
    ...emptyPlatformProbeModels,
    ...Object.fromEntries(
      Object.entries(models).filter(([platform]) => platform in emptyPlatformProbeModels),
    ),
  } as PlatformProbeModelsValues;
}

function platformProbeModelsFromForm(values: PlatformProbeModelsValues): Record<string, string> {
  return Object.fromEntries(
    Object.entries(values)
      .map(([platform, model]) => [platform, model.trim()] as const)
      .filter((entry) => entry[1] !== ""),
  );
}

export function PlatformProbeModelsForm(props: {
  models: Record<string, string>;
  pending: boolean;
  disabled?: boolean;
  onSubmit: (models: Record<string, string>) => void;
}) {
  const form = useForm<PlatformProbeModelsValues>({
    resolver: zodResolver(platformProbeModelsSchema),
    defaultValues: platformProbeModelFormValues(props.models),
  });

  useEffect(() => {
    form.reset(platformProbeModelFormValues(props.models));
  }, [form, props.models]);

  return (
    <form
      className="flex h-full min-h-0 flex-col overflow-hidden"
      onSubmit={form.handleSubmit((values) => props.onSubmit(platformProbeModelsFromForm(values)))}
    >
      <div
        data-slot="settings-scroll"
        className="grid min-h-0 flex-1 content-start gap-x-4 gap-y-3 overflow-y-auto overscroll-contain px-3 py-3 sm:grid-cols-2 lg:grid-cols-3"
      >
        {groupPlatformOptions.map((platform) => {
          const error = form.formState.errors[platform.value]?.message;
          return (
            <label className="grid min-w-0 gap-1.5 text-sm" key={platform.value}>
              <span className="font-medium">{platform.label}</span>
              <Input
                aria-label={`${platform.label} 默认探活模型`}
                aria-invalid={Boolean(error)}
                disabled={props.disabled}
                placeholder="留空则自动选择"
                {...form.register(platform.value)}
              />
              {error ? <span className="text-destructive text-xs">{error}</span> : null}
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
