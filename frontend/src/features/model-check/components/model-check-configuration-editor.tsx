import { RotateCcw } from "lucide-react";
import { useMemo, type FormEventHandler, type ReactElement } from "react";
import type { UseFormReturn } from "react-hook-form";
import type { ModelCheckConfiguration } from "@/api";
import { FieldError } from "@/components/field-error";
import { JsonEditorField } from "@/components/json-editor/form-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { ModelCheckConfigurationForm } from "../lib/model-check-configuration-schema";
import { ModelCheckVersionHistoryDialog } from "./model-check-version-history-dialog";

export function ModelCheckConfigurationEditor(props: {
  configuration: ModelCheckConfiguration;
  form: UseFormReturn<ModelCheckConfigurationForm>;
  pending: boolean;
  onSubmit: FormEventHandler<HTMLFormElement>;
  onReset: () => void;
  onRestore: (versionID: string) => void;
}): ReactElement {
  const current = props.configuration.draft ?? props.configuration.active;
  const counts = useMemo(() => {
    const payload = current.payload;
    const models = new Set([
      ...Object.keys(payload.claude_profiles),
      ...payload.sol_profile.candidate_models,
    ]).size;
    const probes = Object.values(payload.claude_profiles).reduce(
      (total, profile) => total + profile.probes.length,
      payload.sol_profile.quick.length + payload.sol_profile.reserve.length,
    );
    return { models, probes };
  }, [current.payload]);
  const errors = props.form.formState.errors;

  return (
    <form
      id="model-check-configuration-form"
      className="flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-y-auto overscroll-contain"
      onSubmit={props.onSubmit}
    >
      <section
        aria-label="规则版本信息"
        className="flex min-w-0 shrink-0 items-center justify-between gap-3 border-b pb-3"
      >
        <div className="flex min-w-0 flex-1 flex-col gap-1 sm:flex-row sm:items-center sm:gap-3">
          <div className="flex min-w-0 items-center gap-2 sm:max-w-xs">
            <Badge variant={props.configuration.draft ? "warning" : "secondary"}>
              {props.configuration.draft ? "草稿" : "已发布"}
            </Badge>
            <Tooltip>
              <TooltipTrigger render={<code className="min-w-0 truncate text-xs" />}>
                {current.id}
              </TooltipTrigger>
              <TooltipContent className="max-w-sm break-all">{current.id}</TooltipContent>
            </Tooltip>
          </div>
          <p className="text-muted-foreground shrink-0 text-xs tabular-nums">
            {counts.models} 个模型 · {counts.probes} 道题
          </p>
        </div>
        <ModelCheckVersionHistoryDialog
          configuration={props.configuration}
          pending={props.pending}
          onRestore={props.onRestore}
        />
      </section>

      <div className="grid shrink-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3 gap-y-1.5">
        <label htmlFor="model-check-version-note" className="text-sm font-medium">
          版本说明
        </label>
        <Input
          id="model-check-version-note"
          maxLength={200}
          disabled={props.pending}
          aria-invalid={Boolean(errors.note)}
          aria-describedby={errors.note ? "model-check-note-error" : undefined}
          {...props.form.register("note")}
        />
        {errors.note?.message ? (
          <FieldError
            id="model-check-note-error"
            message={errors.note.message}
            className="col-start-2"
          />
        ) : null}
      </div>

      <div className="grid min-h-0 min-w-0 flex-1 grid-rows-[auto_minmax(8rem,1fr)] gap-1.5 has-[.cm-json-search]:grid-rows-[auto_minmax(15rem,1fr)]">
        <div className="flex shrink-0 items-center justify-between gap-3">
          <label htmlFor="model-check-profile-json" className="text-sm font-medium">
            规则与题库 JSON
          </label>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="恢复已保存内容"
                  disabled={props.pending}
                  onClick={props.onReset}
                />
              }
            >
              <RotateCcw aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent>恢复已保存内容</TooltipContent>
          </Tooltip>
        </div>
        <JsonEditorField
          id="model-check-profile-json"
          aria-label="规则与题库 JSON"
          className="h-auto"
          disabled={props.pending}
          aria-invalid={Boolean(errors.payload_json)}
          aria-describedby={errors.payload_json ? "model-check-json-error" : undefined}
          control={props.form.control}
          name="payload_json"
        />
        {errors.payload_json?.message ? (
          <FieldError id="model-check-json-error" message={errors.payload_json.message} />
        ) : null}
      </div>
    </form>
  );
}
