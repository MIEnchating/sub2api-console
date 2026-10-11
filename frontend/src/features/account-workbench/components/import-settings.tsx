import type { ReactElement } from "react";
import { Settings2 } from "lucide-react";
import { Controller, type UseFormReturn } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ImportValues } from "../lib/import-schema";
import { ImportTemplateSelector } from "./import-template-selector";

export function ImportSettings(props: {
  form: UseFormReturn<ImportValues>;
  busy: boolean;
}): ReactElement {
  const form = props.form;
  const values = form.watch();
  const busy = props.busy;
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
        <div className="min-w-0 space-y-1">
          <h2 id="workbench-settings-heading" className="text-sm font-semibold">
            处理设置
          </h2>
          <p className="text-xs text-muted-foreground">选择处理方式与账号配置</p>
        </div>
        <Settings2 aria-hidden="true" className="ml-auto size-5 shrink-0 text-muted-foreground" />
      </div>
      <div className="grid min-w-0 grid-cols-1 gap-4">
        <div className="grid min-w-0 grid-cols-1 gap-2">
          <label htmlFor="workbench-action" className="text-sm">
            处理方式
          </label>
          <Controller
            name="action"
            control={form.control}
            render={({ field }) => (
              <Select value={field.value} disabled={busy} onValueChange={field.onChange}>
                <SelectTrigger id="workbench-action">
                  <SelectValue>
                    {values.action === "import" ? "导入目标站点" : "独立 JSON 输出"}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="import">导入目标站点</SelectItem>
                  <SelectItem value="export">独立 JSON 输出</SelectItem>
                </SelectContent>
              </Select>
            )}
          />
        </div>
        {values.action === "import" && <ImportTemplateSelector form={form} busy={busy} />}
      </div>
      {values.action === "import" && (
        <div className="grid min-w-0 grid-cols-1 gap-2.5 rounded-lg border bg-muted/20 p-3">
          <label className="flex items-center gap-2 text-sm">
            <Controller
              name="check"
              control={form.control}
              render={({ field }) => (
                <Checkbox checked={field.value} disabled={busy} onCheckedChange={field.onChange} />
              )}
            />
            导入后执行智商检测
          </label>
          <p className="text-xs leading-5 text-muted-foreground">
            先套用模板并校验账号，再导入管理平台。检测通过后开启调度；检测未通过时保留账号和分组，不开启调度，可在处理记录中手动启用。关闭检测则导入后直接开启调度。
          </p>
        </div>
      )}
      <div className="grid min-w-0 grid-cols-1 gap-3 border-t pt-4">
        <label className="flex items-center gap-2 text-sm">
          <Controller
            name="proxy_enabled"
            control={form.control}
            render={({ field }) => (
              <Checkbox checked={field.value} disabled={busy} onCheckedChange={field.onChange} />
            )}
          />
          使用登录 / 检测代理
        </label>
        {values.proxy_enabled && (
          <>
            <Input
              aria-label="登录代理地址"
              type="password"
              autoComplete="off"
              disabled={busy}
              placeholder="http://、https:// 或 socks5://"
              {...form.register("proxy_url")}
            />
            {form.formState.errors.proxy_url && (
              <p role="alert" className="text-sm text-destructive">
                {form.formState.errors.proxy_url.message}
              </p>
            )}
          </>
        )}
      </div>
      {values.action === "import" && values.check && (
        <details className="min-w-0 rounded-lg border bg-muted/20 p-3 text-sm">
          <summary className="cursor-pointer rounded-sm text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring">
            检测设置
          </summary>
          <div className="mt-3 grid min-w-0 max-w-sm gap-2">
            <label htmlFor="workbench-model">检测模型</label>
            <Input id="workbench-model" disabled={busy} {...form.register("model")} />
          </div>
        </details>
      )}
    </section>
  );
}
