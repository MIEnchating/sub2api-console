import { useState, type ReactElement } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { FileOutput, RotateCw } from "lucide-react";
import type { AccountStatus } from "@/api";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { exportSchema, type ExportValues } from "../lib/schemas";

export function WorkbenchExportSelection(props: {
  accounts: AccountStatus[];
  disabled: boolean;
  onChange: () => void;
  onSubmit: (ids: string[]) => void;
  onRegenerate?: (ids: string[]) => void;
}): ReactElement {
  const [search, setSearch] = useState("");
  const form = useForm<ExportValues>({
    resolver: zodResolver(exportSchema),
    defaultValues: { account_ids: [] },
  });
  const accounts = props.accounts.filter(
    (account) => account.platform === "openai" && account.account_type === "oauth",
  );
  const needle = search.trim().toLowerCase();
  const visible = accounts.filter((account) =>
    `${account.name} ${account.id}`.toLowerCase().includes(needle),
  );
  return (
    <form
      className="grid min-w-0 gap-3"
      aria-label="选择导出账号"
      onSubmit={form.handleSubmit((values) => props.onSubmit(values.account_ids))}
    >
      <FormField label="搜索账号" htmlFor="workbench-export-search">
        <Input
          id="workbench-export-search"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="账号名称或 ID"
        />
      </FormField>
      <Controller
        name="account_ids"
        control={form.control}
        render={({ field, fieldState }) => {
          const update = (ids: string[]): void => {
            field.onChange(ids);
            props.onChange();
          };
          const allSelected =
            visible.length > 0 && visible.every((item) => field.value.includes(item.id));
          return (
            <FormField label="导出范围" error={fieldState.error?.message}>
              <div className="flex flex-wrap items-center gap-3 py-1 text-sm">
                <label className="flex items-center gap-2">
                  <Checkbox
                    checked={allSelected}
                    disabled={props.disabled || visible.length === 0}
                    onCheckedChange={(checked) =>
                      update(
                        checked
                          ? [...new Set([...field.value, ...visible.map((item) => item.id)])]
                          : field.value.filter((id) => !visible.some((item) => item.id === id)),
                      )
                    }
                  />
                  选择当前结果
                </label>
                <span className="text-muted-foreground">已选 {field.value.length} 个</span>
              </div>
              <fieldset
                aria-label="可导出账号"
                aria-invalid={!!fieldState.error}
                disabled={props.disabled}
                className="grid max-h-64 min-w-0 gap-2 overflow-y-auto rounded-md border p-3 sm:grid-cols-2"
              >
                {visible.map((account) => (
                  <label key={account.id} className="flex min-w-0 items-start gap-2 text-sm">
                    <Checkbox
                      checked={field.value.includes(account.id)}
                      disabled={props.disabled}
                      onCheckedChange={(checked) =>
                        update(
                          checked
                            ? [...field.value, account.id]
                            : field.value.filter((id) => id !== account.id),
                        )
                      }
                    />
                    <span className="min-w-0 wrap-anywhere">
                      {account.name}（ID {account.id}）
                    </span>
                  </label>
                ))}
                {!visible.length && (
                  <p className="text-sm text-muted-foreground">
                    {accounts.length ? "没有匹配的账号" : "暂无可导出的 OpenAI OAuth 账号"}
                  </p>
                )}
              </fieldset>
            </FormField>
          );
        }}
      />
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={props.disabled || accounts.length === 0}>
          <FileOutput aria-hidden="true" />
          预览导出范围
        </Button>
        {props.onRegenerate && (
          <Button
            type="button"
            variant="outline"
            disabled={props.disabled || accounts.length === 0}
            onClick={() =>
              void form.handleSubmit((values) => props.onRegenerate?.(values.account_ids))()
            }
          >
            <RotateCw aria-hidden="true" />
            重新生成授权文件
          </Button>
        )}
      </div>
    </form>
  );
}
