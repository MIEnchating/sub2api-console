import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { RadioTower } from "lucide-react";

import type { NewAPILocalGroup } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { newAPIChannelSchema, parseModelList, type NewAPIChannelValues } from "../lib/schemas";

type Props = {
  groups: NewAPILocalGroup[];
  pending: boolean;
  onSubmit: (payload: {
    name: string;
    sub2api_group_id: string;
    base_url: string;
    service_key: string;
    models: string[];
  }) => void;
};

export function NewAPIChannelForm(props: Props) {
  const form = useForm<NewAPIChannelValues>({
    resolver: zodResolver(newAPIChannelSchema),
    defaultValues: {
      name: "",
      sub2api_group_id: "",
      base_url: "",
      service_key: "",
      models: "",
    },
  });

  function submit(values: NewAPIChannelValues) {
    props.onSubmit({
      name: values.name.trim(),
      sub2api_group_id: values.sub2api_group_id,
      base_url: values.base_url.trim(),
      service_key: values.service_key.trim(),
      models: parseModelList(values.models),
    });
  }

  return (
    <section className="rounded-md border bg-background">
      <div className="border-b px-4 py-3">
        <h2 className="text-sm font-semibold">添加 Sub2API 渠道</h2>
      </div>
      <form className="grid gap-5 p-4 lg:grid-cols-2" onSubmit={form.handleSubmit(submit)}>
        <label className="grid gap-1.5 text-sm">
          <span className="font-medium">渠道名称</span>
          <Input {...form.register("name")} aria-invalid={Boolean(form.formState.errors.name)} />
          {form.formState.errors.name ? (
            <span className="text-destructive text-xs">{form.formState.errors.name.message}</span>
          ) : null}
        </label>
        <label className="grid gap-1.5 text-sm">
          <span className="font-medium">Sub2API 分组</span>
          <Controller
            control={form.control}
            name="sub2api_group_id"
            render={({ field }) => (
              <Select value={field.value || null} onValueChange={field.onChange}>
                <SelectTrigger
                  aria-label="Sub2API 分组"
                  aria-invalid={Boolean(form.formState.errors.sub2api_group_id)}
                >
                  <SelectValue placeholder="选择分组" />
                </SelectTrigger>
                <SelectContent>
                  {props.groups.map((group) => (
                    <SelectItem key={group.id} value={group.id}>
                      {group.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
          {form.formState.errors.sub2api_group_id ? (
            <span className="text-destructive text-xs">
              {form.formState.errors.sub2api_group_id.message}
            </span>
          ) : null}
        </label>
        <label className="grid gap-1.5 text-sm lg:col-span-2">
          <span className="font-medium">Sub2API 服务地址</span>
          <Input
            placeholder="https://sub2api.example.com/v1"
            {...form.register("base_url")}
            aria-invalid={Boolean(form.formState.errors.base_url)}
          />
          {form.formState.errors.base_url ? (
            <span className="text-destructive text-xs">
              {form.formState.errors.base_url.message}
            </span>
          ) : null}
        </label>
        <label className="grid gap-1.5 text-sm lg:col-span-2">
          <span className="font-medium">Sub2API 服务密钥</span>
          <Input
            type="password"
            autoComplete="new-password"
            {...form.register("service_key")}
            aria-invalid={Boolean(form.formState.errors.service_key)}
          />
          {form.formState.errors.service_key ? (
            <span className="text-destructive text-xs">
              {form.formState.errors.service_key.message}
            </span>
          ) : null}
        </label>
        <label className="grid gap-1.5 text-sm lg:col-span-2">
          <span className="font-medium">模型</span>
          <Textarea
            className="min-h-28 font-mono text-xs"
            placeholder={"gpt-5\nclaude-sonnet-4"}
            {...form.register("models")}
            aria-invalid={Boolean(form.formState.errors.models)}
          />
          {form.formState.errors.models ? (
            <span className="text-destructive text-xs">{form.formState.errors.models.message}</span>
          ) : null}
        </label>
        <div className="flex justify-end lg:col-span-2">
          <Button type="submit" disabled={props.pending || props.groups.length === 0}>
            <RadioTower aria-hidden="true" />
            {props.pending ? "正在创建" : "创建渠道"}
          </Button>
        </div>
      </form>
    </section>
  );
}
