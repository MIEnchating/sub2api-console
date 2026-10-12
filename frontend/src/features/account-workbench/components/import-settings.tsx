import type { ReactElement } from "react";
import { Settings2 } from "lucide-react";
import { Controller, type UseFormReturn } from "react-hook-form";
import { Checkbox } from "@/components/ui/checkbox";
import type { ImportValues } from "../lib/import-schema";
import { ImportTemplateSelector } from "./import-template-selector";
import { ImportModelField } from "./import-model-field";
import { ImportProxySelector } from "./import-proxy-selector";

export function ImportSettings(props: {
  form: UseFormReturn<ImportValues>;
  busy: boolean;
  onModelReadyChange: (ready: boolean) => void;
  onProxyReadyChange: (ready: boolean) => void;
}): ReactElement {
  return (
    <section
      aria-labelledby="workbench-settings-heading"
      className="flex min-w-0 flex-col gap-4 rounded-lg border bg-card p-4 shadow-xs sm:p-5"
    >
      <div className="flex items-center gap-3 border-b pb-4">
        <span
          aria-hidden="true"
          className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-sm font-semibold text-primary"
        >
          2
        </span>
        <h2 id="workbench-settings-heading" className="text-sm font-semibold">
          导入设置
        </h2>
        <Settings2 aria-hidden="true" className="ml-auto size-5 shrink-0 text-muted-foreground" />
      </div>
      <ImportTemplateSelector form={props.form} busy={props.busy} />
      <label className="flex items-center gap-2 text-sm">
        <Controller
          name="check"
          control={props.form.control}
          render={({ field }) => (
            <Checkbox
              checked={field.value}
              disabled={props.busy}
              onCheckedChange={field.onChange}
            />
          )}
        />
        导入后执行糖果题检测
      </label>
      <ImportModelField
        form={props.form}
        busy={props.busy}
        onReadyChange={props.onModelReadyChange}
      />
      <ImportProxySelector
        form={props.form}
        busy={props.busy}
        onReadyChange={props.onProxyReadyChange}
      />
    </section>
  );
}
