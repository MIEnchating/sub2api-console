import { Controller, useFieldArray, type UseFormReturn } from "react-hook-form";
import { ArrowUp, ArrowDown, CirclePlus, Trash2 } from "lucide-react";
import { FormField } from "@/App";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle, CardAction, CardContent } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { MultiSelect } from "@/components/multi-select";
import type { KumaResourceList } from "@/api";
import type { ResourceValues } from "../lib/resource-schemas";
import { ResourceTextField, ResourceSelectField, ResourceCheckboxField } from "./resource-fields";
export function StatusPageForm(props: {
  form: UseFormReturn<ResourceValues>;
  options: KumaResourceList;
  pending: boolean;
  editing: boolean;
}) {
  const common = { form: props.form };
  const groups = useFieldArray({
    control: props.form.control,
    name: "status_page.groups",
    keyName: "formKey",
  });
  return (
    <div className="grid gap-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <ResourceTextField {...common} name="status_page.title" label="状态页标题" />
        <ResourceTextField
          {...common}
          name="status_page.slug"
          label="访问路径"
          disabled={props.editing}
          placeholder="service-status"
        />
        <ResourceTextField {...common} name="status_page.description" label="页面说明" multiline />
        <ResourceTextField {...common} name="status_page.footer" label="页脚文案" multiline />
        <ResourceSelectField
          {...common}
          name="status_page.theme"
          label="主题"
          options={{ auto: "跟随系统", light: "浅色", dark: "深色" }}
          disabled={props.pending}
        />
        <FormField
          label="绑定域名（每行一个）"
          error={props.form.formState.errors.status_page?.domains?.message}
        >
          <Controller
            control={props.form.control}
            name="status_page.domains"
            render={({ field }) => (
              <Textarea
                aria-label="绑定域名（每行一个）"
                value={field.value.join("\n")}
                onChange={(e) =>
                  field.onChange(
                    e.target.value ? e.target.value.split("\n").map((v) => v.trim()) : [],
                  )
                }
                placeholder="status.example.com"
              />
            )}
          />
        </FormField>
      </div>
      <div className="flex flex-wrap gap-4">
        <ResourceCheckboxField
          {...common}
          name="status_page.show_tags"
          label="显示标签"
          disabled={props.pending}
        />
        <ResourceCheckboxField
          {...common}
          name="status_page.show_certificate_expiry"
          label="显示证书有效期"
          disabled={props.pending}
        />
        <ResourceCheckboxField
          {...common}
          name="status_page.show_powered_by"
          label="显示 Powered by"
          disabled={props.pending}
        />
      </div>
      {groups.fields.map((group, index) => (
        <Card size="sm" key={group.formKey}>
          <CardHeader>
            <CardTitle>展示分组 {index + 1}</CardTitle>
            <CardAction>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`上移展示分组 ${index + 1}`}
                disabled={props.pending || index === 0}
                onClick={() => groups.move(index, index - 1)}
              >
                <ArrowUp aria-hidden="true" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`下移展示分组 ${index + 1}`}
                disabled={props.pending || index === groups.fields.length - 1}
                onClick={() => groups.move(index, index + 1)}
              >
                <ArrowDown aria-hidden="true" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`移除展示分组 ${index + 1}`}
                disabled={props.pending}
                onClick={() => groups.remove(index)}
              >
                <Trash2 aria-hidden="true" />
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent className="grid gap-3">
            <ResourceTextField
              {...common}
              name={`status_page.groups.${index}.name`}
              label={`分组 ${index + 1} 名称`}
            />
            <FormField label={`分组 ${index + 1} 监控项`}>
              <Controller
                control={props.form.control}
                name={`status_page.groups.${index}.monitorList`}
                render={({ field }) => (
                  <MultiSelect
                    ariaLabel={`分组 ${index + 1} 监控项`}
                    title="选择公开展示的监控项"
                    options={props.options.monitors.map((m) => ({
                      label: m.name,
                      value: String(m.id),
                    }))}
                    selected={field.value.map((m) => String(m.id))}
                    onChange={(values) =>
                      field.onChange(
                        values.map(
                          (v) =>
                            field.value.find((m) => m.id === Number(v)) ?? {
                              id: Number(v),
                              sendUrl: false,
                            },
                        ),
                      )
                    }
                    disabled={props.pending}
                  />
                )}
              />
            </FormField>
            <Controller
              control={props.form.control}
              name={`status_page.groups.${index}.monitorList`}
              render={({ field }) => (
                <label className="flex items-center gap-2 text-sm">
                  <Checkbox
                    aria-label={`分组 ${index + 1} 公开监控地址`}
                    checked={field.value.length > 0 && field.value.every((m) => m.sendUrl)}
                    onCheckedChange={(checked) =>
                      field.onChange(field.value.map((m) => ({ ...m, sendUrl: checked })))
                    }
                    disabled={props.pending || field.value.length === 0}
                  />
                  公开监控地址
                </label>
              )}
            />
          </CardContent>
        </Card>
      ))}
      <Button
        type="button"
        variant="outline"
        className="justify-self-start"
        disabled={props.pending}
        onClick={() => groups.append({ name: "服务状态", monitorList: [] })}
      >
        <CirclePlus aria-hidden="true" />
        添加展示分组
      </Button>
    </div>
  );
}
