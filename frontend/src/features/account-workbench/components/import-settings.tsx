import type { ReactElement } from "react";
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
import type { TemplateLibrary } from "../types";

export function ImportSettings(props: {
  form: UseFormReturn<ImportValues>;
  templates: TemplateLibrary | undefined;
  loading: boolean;
  busy: boolean;
}): ReactElement {
  const form = props.form;
  const values = form.watch();
  const busy = props.busy;
  return (
    <section
      aria-labelledby="workbench-settings-heading"
      className="flex min-w-0 flex-col gap-4 rounded-xl border bg-card p-4"
    >
      <div className="flex items-center gap-2.5 border-b pb-3">
        <span
          aria-hidden="true"
          className="flex size-6 shrink-0 items-center justify-center rounded-md bg-primary/10 text-xs font-semibold text-primary"
        >
          2
        </span>
        <h2 id="workbench-settings-heading" className="text-sm font-semibold">
          处理设置
        </h2>
      </div>
      <div className="grid gap-3">
        <div className="grid gap-2">
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
        {values.action === "import" && (
          <div className="grid gap-2">
            <label htmlFor="workbench-template" className="text-sm">
              配置模板
            </label>
            <Controller
              name="template_id"
              control={form.control}
              render={({ field }) => (
                <Select
                  value={field.value || "__auto__"}
                  disabled={busy || props.loading}
                  onValueChange={(value) => field.onChange(value === "__auto__" ? "" : value)}
                >
                  <SelectTrigger id="workbench-template">
                    <SelectValue>
                      {props.templates?.items.find((item) => item.id === field.value)?.name ||
                        "首选模板 / 自动匹配"}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__auto__">首选模板 / 自动匹配</SelectItem>
                    {props.templates?.items.map((item) => (
                      <SelectItem key={item.id} value={item.id}>
                        {item.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </div>
        )}
      </div>
      {values.action === "import" && (
        <div className="grid gap-3 rounded-lg border bg-muted/20 p-3">
          {(
            [
              { name: "check", label: "导入前执行 Sol 检测" },
              { name: "promote", label: "导入完成后启用账号" },
            ] as const
          ).map((option) => (
            <label key={option.name} className="flex items-center gap-2 text-sm">
              <Controller
                name={option.name}
                control={form.control}
                render={({ field }) => (
                  <Checkbox
                    checked={field.value}
                    disabled={busy}
                    onCheckedChange={field.onChange}
                  />
                )}
              />
              {option.label}
            </label>
          ))}
          <p className="text-xs text-muted-foreground">
            检测出错时停止导入；检测完成后的不匹配或证据不足结论不影响导入。
          </p>
        </div>
      )}
      <div className="grid gap-3 rounded-lg border bg-muted/20 p-3">
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
        <details className="rounded-lg border bg-muted/20 p-3 text-sm">
          <summary className="cursor-pointer text-muted-foreground">检测设置</summary>
          <div className="mt-3 grid max-w-sm gap-2">
            <label htmlFor="workbench-model">检测模型</label>
            <Input id="workbench-model" disabled={busy} {...form.register("model")} />
          </div>
        </details>
      )}
    </section>
  );
}
