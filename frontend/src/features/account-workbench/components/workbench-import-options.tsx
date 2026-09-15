import type { ReactElement } from "react";
import { Controller, type UseFormReturn } from "react-hook-form";
import type { WorkbenchTemplate } from "@/api";
import { FormField } from "@/components/form-field";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ImportValues } from "../lib/schemas";

export function WorkbenchImportOptions(props: {
  form: UseFormReturn<ImportValues>;
  templates: WorkbenchTemplate[];
  exportOnly: boolean;
  disabled: boolean;
  onChange: () => void;
}): ReactElement {
  const checking = props.form.watch("check_after_import");
  const modelError = props.form.formState.errors.model?.message;
  return (
    <section
      aria-label="导入选项"
      className="grid min-w-0 self-start gap-4 border-t pt-5 @3xl/import:border-t-0 @3xl/import:border-l @3xl/import:pt-0 @3xl/import:pl-5"
    >
      <FormField label="配置模板">
        <Controller
          control={props.form.control}
          name="template_id"
          render={({ field }) => (
            <Select
              value={field.value || "auto"}
              disabled={props.disabled}
              onValueChange={(value) => {
                props.onChange();
                field.onChange(value === "auto" ? "" : value);
              }}
            >
              <SelectTrigger aria-label="配置模板" className="min-w-0">
                <SelectValue>
                  <span className="min-w-0 truncate">
                    {props.templates.find((item) => item.id === field.value)?.name ||
                      "自动匹配模板"}
                  </span>
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="auto">自动匹配模板</SelectItem>
                {props.templates.map((item) => (
                  <SelectItem key={item.id} value={item.id}>
                    {item.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
      </FormField>
      {!props.exportOnly && (
        <div className="grid min-w-0 gap-3 border-t pt-4">
          <label className="flex min-w-0 items-start gap-2 text-sm">
            <Controller
              control={props.form.control}
              name="check_after_import"
              render={({ field }) => (
                <Checkbox
                  checked={field.value}
                  disabled={props.disabled}
                  onCheckedChange={(value) => {
                    props.onChange();
                    field.onChange(value);
                  }}
                />
              )}
            />
            <span className="min-w-0 wrap-anywhere">导入后检测（会产生模型调用用量）</span>
          </label>
          {(checking || modelError) && (
            <FormField
              label="检测模型"
              htmlFor="workbench-import-model"
              error={modelError}
              reserveErrorSpace={false}
            >
              <Input
                id="workbench-import-model"
                disabled={props.disabled}
                aria-invalid={!!modelError}
                placeholder="填写已支持的模型名称"
                {...props.form.register("model")}
              />
            </FormField>
          )}
        </div>
      )}
      <p className="text-sm leading-6 text-muted-foreground wrap-anywhere">
        {props.exportOnly
          ? "确认后生成服务器私有 JSON 文件，不写入线上账号。"
          : "先预览账号与配置，确认后再导入。"}
      </p>
    </section>
  );
}
