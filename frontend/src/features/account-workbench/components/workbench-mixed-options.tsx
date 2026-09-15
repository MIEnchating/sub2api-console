import { useState, type ReactElement, type ReactNode } from "react";
import { ChevronDown, ChevronRight, FileJson, Plus, Upload } from "lucide-react";
import { Controller, useFormContext } from "react-hook-form";
import type { WorkbenchTemplate } from "@/api";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { MixedRunValues } from "../lib/mixed-run-schema";
import { WorkbenchTemplateDialog } from "./workbench-template-dialog";
import { WorkbenchSelectedTemplate } from "./workbench-selected-template";

export function WorkbenchMixedOptions(props: {
  local: boolean;
  templates: WorkbenchTemplate[];
  disabled: boolean;
  children?: ReactNode;
  onTemplateChange: (id: string) => void;
}): ReactElement {
  const form = useFormContext<MixedRunValues>();
  const exportOnly = form.watch("export_only");
  const templateId = form.watch("template_id");
  const selectedTemplate = props.templates.find((item) => item.id === templateId);
  const [expanded, setExpanded] = useState(false);
  const [sourceOpen, setSourceOpen] = useState(false);
  const hasError = Object.keys(form.formState.errors).some(
    (name) => name === "proxy_url" || name === "model",
  );
  const showAdvanced = expanded || hasError;
  const ExpandIcon = showAdvanced ? ChevronDown : ChevronRight;
  return (
    <section
      aria-label="导入选项"
      className="grid min-w-0 self-start gap-4 border-t pt-5 @3xl/mixed:border-t-0 @3xl/mixed:border-l @3xl/mixed:pt-0 @3xl/mixed:pl-5"
    >
      {!props.local && !exportOnly && (
        <>
          <FormField label="配置模板">
            <Controller
              control={form.control}
              name="template_id"
              render={({ field }) => (
                <Select
                  value={field.value || "auto"}
                  disabled={props.disabled}
                  onValueChange={(value) =>
                    props.onTemplateChange(value === "auto" ? "" : (value ?? ""))
                  }
                >
                  <SelectTrigger aria-label="配置模板" className="min-w-0">
                    <SelectValue>
                      <span className="min-w-0 truncate">
                        {props.templates.find((item) => item.id === field.value)?.name ??
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
          <Button
            type="button"
            variant="ghost"
            className="justify-start"
            disabled={props.disabled}
            onClick={() => setSourceOpen(true)}
          >
            <Plus aria-hidden="true" />
            读取线上配置
          </Button>
          {selectedTemplate && <WorkbenchSelectedTemplate template={selectedTemplate} />}
        </>
      )}
      <Controller
        control={form.control}
        name="export_only"
        render={({ field }) => (
          <SegmentedControl aria-label="处理方式" className="grid w-full grid-cols-2">
            <SegmentedControlItem
              selected={!field.value}
              disabled={props.disabled || props.local}
              onClick={() => field.onChange(false)}
            >
              <Upload aria-hidden="true" />
              导入站点
            </SegmentedControlItem>
            <SegmentedControlItem
              selected={field.value}
              disabled={props.disabled}
              onClick={() => field.onChange(true)}
            >
              <FileJson aria-hidden="true" />
              仅导出 JSON
            </SegmentedControlItem>
          </SegmentedControl>
        )}
      />
      <Button
        type="button"
        variant="ghost"
        className="justify-start"
        aria-expanded={showAdvanced}
        aria-controls="mixed-login-options"
        onClick={() => setExpanded(!expanded)}
      >
        <ExpandIcon aria-hidden="true" />
        高级设置
      </Button>
      <div id="mixed-login-options" hidden={!showAdvanced}>
        <div className="grid min-w-0 gap-3">
          <FormField
            label="登录 / 检测代理"
            htmlFor="mixed-run-proxy"
            error={form.formState.errors.proxy_url?.message}
            reserveErrorSpace={false}
          >
            <Input
              id="mixed-run-proxy"
              type="password"
              autoComplete="off"
              placeholder="直连"
              aria-invalid={!!form.formState.errors.proxy_url}
              {...form.register("proxy_url")}
            />
          </FormField>
          {!exportOnly && (
            <FormField
              label="检测模型"
              htmlFor="mixed-run-model"
              error={form.formState.errors.model?.message}
              reserveErrorSpace={false}
            >
              <Input
                id="mixed-run-model"
                disabled={props.disabled}
                placeholder="gpt-5.6-sol"
                aria-invalid={!!form.formState.errors.model}
                {...form.register("model")}
              />
            </FormField>
          )}
        </div>
      </div>
      {props.children}
      {sourceOpen && (
        <WorkbenchTemplateDialog
          onClose={() => setSourceOpen(false)}
          onSaved={(template) => form.setValue("template_id", template.id)}
        />
      )}
    </section>
  );
}
