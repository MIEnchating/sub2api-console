import { useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import type { NewAPIChannel } from "@/api";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  channelMaintenanceSchema,
  parseChannelModelText,
  type ChannelMaintenanceValues,
} from "../lib/channel-maintenance-schema";
import {
  channelModelImpact,
  type ChannelModelAction,
  type ChannelModelSelection,
} from "../lib/channel-model-change";
import { ChannelModelSelection as ChannelModelSelector } from "./channel-model-selection";
import { ChannelImpactPreview } from "./channel-impact-preview";

export function ChannelMaintenanceDialog(props: {
  channels: NewAPIChannel[];
  action: ChannelModelAction;
  pending: boolean;
  onClose: () => void;
  onSubmit: (input: ChannelModelSelection) => Promise<void>;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const [preview, setPreview] = useState<string[] | null>(null);
  const form = useForm<ChannelMaintenanceValues>({
    resolver: zodResolver(channelMaintenanceSchema),
    defaultValues: { modelText: "" },
  });
  const label = props.action === "add" ? "上架模型" : "下架模型";
  const models = useMemo(
    () => [...new Set(props.channels.flatMap((channel) => channel.models))],
    [props.channels],
  );
  const emptied = props.channels.filter(
    (channel) =>
      channelModelImpact(channel, "remove", selected).remaining === 0 && channel.models.length > 0,
  );
  function prepare(values: ChannelMaintenanceValues): void {
    const models = parseChannelModelText(values.modelText).filter((model) =>
      props.channels.some((channel) => !channel.models.includes(model)),
    );
    if (models.length === 0) {
      form.setError("modelText", { message: "请输入尚未上架的模型名称" });
      return;
    }
    if (models.length > 1000) {
      form.setError("modelText", { message: "每次最多上架 1000 个模型" });
      return;
    }
    setPreview(models);
  }
  async function submit(): Promise<void> {
    if (!preview || props.pending) return;
    try {
      await props.onSubmit({ action: props.action, models: preview });
    } catch {
      /* The mutation reports the failure. */
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !props.pending) props.onClose();
      }}
    >
      <DialogContent width="progress" height="large">
        <DialogHeader>
          <DialogTitle>{preview ? `确认${label}` : label}</DialogTitle>
          <DialogDescription className="break-all">
            已选择 {props.channels.length} 个渠道
            {props.channels.length === 1
              ? ` · ${props.channels[0].name} · ID ${props.channels[0].id}`
              : "，确认前请核对每个渠道的影响范围。"}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid min-w-0 gap-3">
          {preview && (
            <ChannelImpactPreview
              channels={props.channels}
              action={props.action}
              models={preview}
            />
          )}
          {!preview && props.action === "add" && (
            <form
              id="channel-model-maintenance"
              onSubmit={form.handleSubmit(prepare)}
              className="grid gap-2"
            >
              <label htmlFor="channel-model-names" className="text-sm font-medium">
                上架模型名称
              </label>
              <Textarea
                id="channel-model-names"
                aria-invalid={Boolean(form.formState.errors.modelText)}
                aria-describedby="channel-model-help"
                placeholder="每行一个模型，或用逗号分隔"
                {...form.register("modelText")}
              />
              <p id="channel-model-help" className="text-muted-foreground text-xs">
                请填写上游支持的准确模型名称，支持一次输入多个模型，已上架的模型自动跳过。
              </p>
              {form.formState.errors.modelText && (
                <p role="alert" className="text-destructive text-sm">
                  {form.formState.errors.modelText.message}
                </p>
              )}
            </form>
          )}
          {!preview && props.action === "remove" && (
            <>
              <ChannelModelSelector models={models} selected={selected} onChange={setSelected} />
              <p className="text-muted-foreground text-xs">
                已选 {selected.length} 个模型。每个渠道至少保留一个模型。
              </p>
              {emptied.length > 0 && (
                <p role="alert" className="text-destructive break-all text-xs">
                  当前选择会清空 {emptied.map((channel) => channel.name).join("、")}
                  ，请保留至少一个模型。
                </p>
              )}
            </>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" disabled={props.pending} onClick={props.onClose}>
            取消
          </Button>
          {preview && (
            <>
              <Button variant="outline" disabled={props.pending} onClick={() => setPreview(null)}>
                返回修改
              </Button>
              <Button disabled={props.pending} onClick={() => void submit()}>
                {props.pending ? "正在提交…" : `确认${label}`}
              </Button>
            </>
          )}
          {!preview && props.action === "add" && (
            <Button type="submit" form="channel-model-maintenance">
              预览变更
            </Button>
          )}
          {!preview && props.action === "remove" && (
            <Button
              disabled={selected.length === 0 || emptied.length > 0 || selected.length > 1000}
              onClick={() => setPreview(selected)}
            >
              预览变更
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
