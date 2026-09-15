import { ContentRetry } from "@/components/content-retry";
import { Tabs } from "@base-ui/react/tabs";
import { ContentLoading } from "@/components/content-loading";
import { QueryErrorToast } from "@/components/query-error-toast";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save, Send, Trash2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { notifyOperationError } from "@/lib/operation-feedback";

import {
  api,
  type ModelCheckConfiguration,
  type ModelCheckConfigurationVersion,
  type ModelCheckProfilePayload,
} from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { ModelCheckRuleBrowser } from "./model-check-rule-browser";
import { ModelCheckConfigurationEditor } from "./model-check-configuration-editor";

import {
  modelCheckConfigurationSchema,
  type ModelCheckConfigurationForm,
} from "../lib/model-check-configuration-schema";

type ModelCheckConfigurationDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

function editableVersion(configuration: ModelCheckConfiguration): ModelCheckConfigurationVersion {
  return configuration.draft ?? configuration.active;
}

function expectedFingerprint(configuration: ModelCheckConfiguration): string {
  return editableVersion(configuration).fingerprint;
}

function payloadText(version: ModelCheckConfigurationVersion): string {
  return JSON.stringify(version.payload, null, 2);
}

export function ModelCheckConfigurationDialog(props: ModelCheckConfigurationDialogProps) {
  const queryClient = useQueryClient();
  const [view, setView] = useState("rules");
  const [publishFingerprint, setPublishFingerprint] = useState<string | null>(null);
  const [discardFingerprint, setDiscardFingerprint] = useState<string | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<{ id: string; fingerprint: string } | null>(
    null,
  );
  const editingFingerprint = useRef<string | null>(null);
  const form = useForm<ModelCheckConfigurationForm>({
    resolver: zodResolver(modelCheckConfigurationSchema),
    defaultValues: { note: "", payload_json: "" },
  });
  const configuration = useQuery({
    queryKey: ["model-check-configuration"],
    queryFn: api.modelCheckConfiguration,
    enabled: props.open,
  });
  const isDirty = form.formState.isDirty;

  useEffect(() => {
    if (!props.open) setView("rules");
  }, [props.open]);

  useEffect(() => {
    if (!configuration.data || isDirty) return;
    const version = editableVersion(configuration.data);
    editingFingerprint.current = version.fingerprint;
    form.reset({ note: version.note, payload_json: payloadText(version) });
  }, [configuration.data, form, isDirty]);

  function applyConfiguration(value: ModelCheckConfiguration, message: string) {
    queryClient.setQueryData(["model-check-configuration"], value);
    const version = editableVersion(value);
    editingFingerprint.current = version.fingerprint;
    form.reset({ note: version.note, payload_json: payloadText(version) });
    toast.success(message);
  }

  const save = useMutation({
    mutationFn: (values: ModelCheckConfigurationForm) => {
      if (!editingFingerprint.current) throw new Error("检测规则尚未读取完成");
      const payload = JSON.parse(values.payload_json) as ModelCheckProfilePayload;
      return api.saveModelCheckDraft({
        expected_fingerprint: editingFingerprint.current,
        note: values.note.trim(),
        payload,
      });
    },
    onSuccess: (value) => applyConfiguration(value, "检测规则草稿已保存"),
    onError: (error) => notifyOperationError(error, "检测规则草稿保存失败"),
  });
  const publish = useMutation({
    mutationFn: (fingerprint: string) => api.publishModelCheckDraft(fingerprint),
    onSuccess: (value) => {
      setPublishFingerprint(null);
      applyConfiguration(value, "检测规则已发布");
      void queryClient.invalidateQueries({ queryKey: ["model-check-capabilities"] });
    },
    onError: (error) => notifyOperationError(error, "检测规则发布失败"),
  });
  const discard = useMutation({
    mutationFn: (fingerprint: string) => api.discardModelCheckDraft(fingerprint),
    onSuccess: (value) => {
      setDiscardFingerprint(null);
      applyConfiguration(value, "检测规则草稿已删除");
    },
    onError: (error) => notifyOperationError(error, "检测规则草稿删除失败"),
  });
  const restore = useMutation({
    mutationFn: (target: { id: string; fingerprint: string }) => {
      return api.restoreModelCheckVersion(target.id, target.fingerprint, `恢复自版本 ${target.id}`);
    },
    onSuccess: (value) => {
      setRestoreTarget(null);
      applyConfiguration(value, "历史版本已恢复为草稿");
    },
    onError: (error) => notifyOperationError(error, "历史版本恢复失败"),
  });

  const pending = save.isPending || publish.isPending || discard.isPending || restore.isPending;
  const submit = form.handleSubmit((values) => save.mutate(values));

  return (
    <>
      <Dialog open={props.open} onOpenChange={(open) => !pending && props.onOpenChange(open)}>
        <DialogContent
          width="wide"
          height="tall"
          className="flex h-[min(44rem,calc(100svh-2rem))] flex-col overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>检测规则与题库</DialogTitle>
            <DialogDescription>
              根据固定题目的回答特征判断更接近哪个模型，仅供行为对比，不能证明模型身份。
            </DialogDescription>
          </DialogHeader>
          <Tabs.Root
            value={view}
            onValueChange={(value) => setView(String(value))}
            className="flex min-h-0 flex-1 flex-col gap-3"
          >
            <Tabs.List aria-label="检测规则视图" className="flex shrink-0 gap-4 border-b">
              <Tabs.Tab
                value="rules"
                className="border-b-2 border-transparent py-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
              >
                规则与题目
              </Tabs.Tab>
              <Tabs.Tab
                value="advanced"
                className="border-b-2 border-transparent py-2 text-sm data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
              >
                高级设置
              </Tabs.Tab>
            </Tabs.List>
            <DialogBody className="relative min-w-0 flex-1 overflow-hidden">
              {configuration.isLoading ? <ContentLoading label="正在读取检测规则" /> : null}
              {configuration.error ? (
                <>
                  <QueryErrorToast error={configuration.error} fallback="检测规则读取失败" />
                  {!configuration.data && (
                    <ContentRetry
                      onRetry={() => void configuration.refetch()}
                      pending={configuration.isFetching}
                    />
                  )}
                </>
              ) : null}
              {configuration.data ? (
                <>
                  {/* 保留面板布局，避免大段 JSON 在切换时重新排版；Base UI 的 inert 阻止非当前面板交互。 */}
                  <Tabs.Panel
                    value="rules"
                    keepMounted
                    hidden={false}
                    aria-hidden={view !== "rules"}
                    className="absolute inset-0 min-h-0 min-w-0 data-[hidden]:invisible"
                  >
                    {configuration.data ? (
                      <ModelCheckRuleBrowser configuration={configuration.data} />
                    ) : null}
                  </Tabs.Panel>
                  <Tabs.Panel
                    value="advanced"
                    keepMounted
                    hidden={false}
                    aria-hidden={view !== "advanced"}
                    className="absolute inset-0 flex min-h-0 min-w-0 flex-col gap-3 data-[hidden]:invisible"
                  >
                    <ModelCheckConfigurationEditor
                      configuration={configuration.data}
                      form={form}
                      pending={pending}
                      onSubmit={submit}
                      onRestore={(id) =>
                        setRestoreTarget({
                          id,
                          fingerprint: expectedFingerprint(configuration.data),
                        })
                      }
                      onReset={() => {
                        const version = editableVersion(configuration.data);
                        editingFingerprint.current = version.fingerprint;
                        form.reset({ note: version.note, payload_json: payloadText(version) });
                      }}
                    />
                    <DialogFooter className="m-0 shrink-0 flex-row flex-nowrap items-center justify-end rounded-none bg-transparent p-0 pt-3">
                      {configuration.data.draft ? (
                        <Tooltip>
                          <TooltipTrigger
                            render={
                              <Button
                                type="button"
                                variant="ghost"
                                size="icon"
                                className="text-destructive mr-auto"
                                aria-label="删除草稿"
                                disabled={pending}
                                onClick={() =>
                                  setDiscardFingerprint(configuration.data.draft!.fingerprint)
                                }
                              />
                            }
                          >
                            <Trash2 aria-hidden="true" />
                          </TooltipTrigger>
                          <TooltipContent>删除草稿</TooltipContent>
                        </Tooltip>
                      ) : null}
                      <Button
                        type="submit"
                        form="model-check-configuration-form"
                        variant="outline"
                        disabled={pending}
                      >
                        <Save aria-hidden="true" />
                        {save.isPending ? "保存中..." : "保存草稿"}
                      </Button>
                      <Button
                        type="button"
                        disabled={!configuration.data.draft || pending || form.formState.isDirty}
                        onClick={() => setPublishFingerprint(configuration.data.draft!.fingerprint)}
                      >
                        <Send aria-hidden="true" />
                        发布生效
                      </Button>
                    </DialogFooter>
                  </Tabs.Panel>
                </>
              ) : null}
            </DialogBody>
          </Tabs.Root>
        </DialogContent>
      </Dialog>

      <ConfirmActionDialog
        open={publishFingerprint !== null}
        title="发布检测规则"
        description="发布后，新创建的模型检测任务将使用该草稿；已经排队或运行中的任务继续使用原规则。"
        confirmLabel="确认发布"
        pendingLabel="发布中..."
        pending={publish.isPending}
        onOpenChange={(open) => {
          if (!open) setPublishFingerprint(null);
        }}
        onConfirm={() => {
          if (publishFingerprint) publish.mutate(publishFingerprint);
        }}
      />
      <ConfirmActionDialog
        open={discardFingerprint !== null}
        title="删除规则草稿"
        description="未发布的题库和规则修改将被删除，当前已发布版本不会受到影响。"
        confirmLabel="确认删除"
        pendingLabel="删除中..."
        pending={discard.isPending}
        onOpenChange={(open) => {
          if (!open) setDiscardFingerprint(null);
        }}
        onConfirm={() => {
          if (discardFingerprint) discard.mutate(discardFingerprint);
        }}
      />
      <ConfirmActionDialog
        open={restoreTarget !== null}
        title="恢复历史规则"
        description="所选历史版本将复制为新草稿，不会立即影响模型检测。"
        confirmLabel="恢复为草稿"
        pendingLabel="恢复中..."
        pending={restore.isPending}
        onOpenChange={(open) => !open && setRestoreTarget(null)}
        onConfirm={() => {
          if (restoreTarget) restore.mutate(restoreTarget);
        }}
      />
    </>
  );
}
