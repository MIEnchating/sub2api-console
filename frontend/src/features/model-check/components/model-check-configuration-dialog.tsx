import { ContentRetry } from "@/components/content-retry";
import { ContentLoading } from "@/components/content-loading";
import { QueryErrorToast } from "@/components/query-error-toast";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArchiveRestore, RefreshCw, Save, Send, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";

import {
  api,
  type ModelCheckConfiguration,
  type ModelCheckConfigurationVersion,
  type ModelCheckProfilePayload,
} from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

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

function profileCounts(payload: ModelCheckProfilePayload): { claude: number; probes: number } {
  let probes = payload.sol_profile.quick.length + payload.sol_profile.reserve.length;
  const claudeProfiles = Object.values(payload.claude_profiles);
  for (const profile of claudeProfiles) probes += profile.probes.length;
  return { claude: claudeProfiles.length, probes };
}

function shortFingerprint(value: string): string {
  return value.length <= 16 ? value : `${value.slice(0, 8)}...${value.slice(-8)}`;
}

function displayTime(value: string | null): string {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}

export function ModelCheckConfigurationDialog(props: ModelCheckConfigurationDialogProps) {
  const queryClient = useQueryClient();
  const [publishConfirmOpen, setPublishConfirmOpen] = useState(false);
  const [discardConfirmOpen, setDiscardConfirmOpen] = useState(false);
  const [restoreVersionID, setRestoreVersionID] = useState<string | null>(null);
  const form = useForm<ModelCheckConfigurationForm>({
    resolver: zodResolver(modelCheckConfigurationSchema),
    defaultValues: { note: "", payload_json: "" },
  });
  const configuration = useQuery({
    queryKey: ["model-check-configuration"],
    queryFn: api.modelCheckConfiguration,
    enabled: props.open,
  });

  useEffect(() => {
    if (!configuration.data) return;
    const version = editableVersion(configuration.data);
    form.reset({ note: version.note, payload_json: payloadText(version) });
  }, [configuration.data, form]);

  function applyConfiguration(value: ModelCheckConfiguration, message: string) {
    queryClient.setQueryData(["model-check-configuration"], value);
    const version = editableVersion(value);
    form.reset({ note: version.note, payload_json: payloadText(version) });
    toast.success(message);
  }

  const save = useMutation({
    mutationFn: (values: ModelCheckConfigurationForm) => {
      if (!configuration.data) throw new Error("画像配置尚未读取完成");
      const payload = JSON.parse(values.payload_json) as ModelCheckProfilePayload;
      return api.saveModelCheckDraft({
        expected_fingerprint: expectedFingerprint(configuration.data),
        note: values.note.trim(),
        payload,
      });
    },
    onSuccess: (value) => applyConfiguration(value, "画像草稿已保存"),
    onError: (error) => toast.error(error instanceof Error ? error.message : "画像草稿保存失败"),
  });
  const publish = useMutation({
    mutationFn: () => {
      if (!configuration.data?.draft) throw new Error("没有可发布的画像草稿");
      return api.publishModelCheckDraft(configuration.data.draft.fingerprint);
    },
    onSuccess: (value) => {
      setPublishConfirmOpen(false);
      applyConfiguration(value, "画像版本已发布");
      void queryClient.invalidateQueries({ queryKey: ["model-check-capabilities"] });
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "画像发布失败"),
  });
  const discard = useMutation({
    mutationFn: () => {
      if (!configuration.data?.draft) throw new Error("没有可删除的画像草稿");
      return api.discardModelCheckDraft(configuration.data.draft.fingerprint);
    },
    onSuccess: (value) => {
      setDiscardConfirmOpen(false);
      applyConfiguration(value, "画像草稿已删除");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "画像草稿删除失败"),
  });
  const restore = useMutation({
    mutationFn: (versionID: string) => {
      if (!configuration.data) throw new Error("画像配置尚未读取完成");
      return api.restoreModelCheckVersion(
        versionID,
        expectedFingerprint(configuration.data),
        `恢复自版本 ${versionID}`,
      );
    },
    onSuccess: (value) => {
      setRestoreVersionID(null);
      applyConfiguration(value, "历史版本已恢复为草稿");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "历史版本恢复失败"),
  });

  const current = configuration.data ? editableVersion(configuration.data) : null;
  const counts = current ? profileCounts(current.payload) : null;
  const pending = save.isPending || publish.isPending || discard.isPending || restore.isPending;
  const submit = form.handleSubmit((values) => save.mutate(values));

  return (
    <>
      <Dialog open={props.open} onOpenChange={(open) => !pending && props.onOpenChange(open)}>
        <DialogContent
          width="wide"
          height="tall"
          className="grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>检测题库与行为画像</DialogTitle>
            <DialogDescription>
              当前生效版本 {configuration.data?.active.id ?? "读取中"}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="overflow-y-auto">
            {configuration.isLoading ? <ContentLoading label="正在读取画像配置" /> : null}
            {configuration.error ? (
              <>
                <QueryErrorToast error={configuration.error} fallback="画像配置读取失败" />
                {!configuration.data && (
                  <ContentRetry
                    onRetry={() => void configuration.refetch()}
                    pending={configuration.isFetching}
                  />
                )}
              </>
            ) : null}
            {configuration.data && current && counts ? (
              <form id="model-check-configuration-form" className="grid gap-4" onSubmit={submit}>
                <section className="grid gap-3 border-b pb-4" aria-label="画像版本信息">
                  <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                    <VersionMetric label="编辑版本" value={current.id} />
                    <VersionMetric
                      label="状态"
                      value={configuration.data.draft ? "草稿" : "已发布"}
                    />
                    <VersionMetric label="Claude 画像" value={`${counts.claude} 个`} />
                    <VersionMetric label="检测题目" value={`${counts.probes} 道`} />
                  </div>
                  <p className="text-muted-foreground text-xs">
                    内容指纹{" "}
                    <Tooltip>
                      <TooltipTrigger render={<code />}>
                        {shortFingerprint(current.fingerprint)}
                      </TooltipTrigger>
                      <TooltipContent className="max-w-md break-all">
                        {current.fingerprint}
                      </TooltipContent>
                    </Tooltip>
                  </p>
                </section>

                <div className="grid gap-2">
                  <label htmlFor="model-check-version-note" className="text-sm font-medium">
                    版本说明
                  </label>
                  <Input
                    id="model-check-version-note"
                    maxLength={200}
                    disabled={pending}
                    aria-invalid={Boolean(form.formState.errors.note)}
                    {...form.register("note")}
                  />
                  {form.formState.errors.note ? (
                    <p className="text-destructive text-xs" role="alert">
                      {form.formState.errors.note.message}
                    </p>
                  ) : null}
                </div>

                <div className="grid min-h-96 gap-2">
                  <div className="flex items-center justify-between gap-3">
                    <label htmlFor="model-check-profile-json" className="text-sm font-medium">
                      题库与画像 JSON
                    </label>
                    <Button
                      type="button"
                      variant="ghost"
                      disabled={pending}
                      onClick={() => {
                        const version = editableVersion(configuration.data);
                        form.reset({ note: version.note, payload_json: payloadText(version) });
                      }}
                    >
                      <RefreshCw aria-hidden="true" />
                      恢复已保存内容
                    </Button>
                  </div>
                  <Textarea
                    id="model-check-profile-json"
                    className="min-h-96 resize-y font-mono text-xs leading-5"
                    spellCheck={false}
                    disabled={pending}
                    aria-invalid={Boolean(form.formState.errors.payload_json)}
                    {...form.register("payload_json")}
                  />
                  {form.formState.errors.payload_json ? (
                    <p className="text-destructive text-xs" role="alert">
                      {form.formState.errors.payload_json.message}
                    </p>
                  ) : null}
                </div>

                <section
                  className="grid gap-2 border-t pt-4"
                  aria-labelledby="profile-history-title"
                >
                  <h3 id="profile-history-title" className="text-sm font-medium">
                    已发布版本历史
                  </h3>
                  {configuration.data.history.length === 0 ? (
                    <p className="text-muted-foreground text-sm">暂无历史版本</p>
                  ) : (
                    <div className="divide-y rounded-md border">
                      {configuration.data.history.map((version) => (
                        <div
                          key={version.id}
                          className="grid gap-2 px-3 py-2.5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
                        >
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium">
                              {version.note || version.id}
                            </p>
                            <p className="text-muted-foreground mt-0.5 text-xs">
                              {displayTime(version.published_at)} · {version.probe_count} 道题 ·{" "}
                              {shortFingerprint(version.fingerprint)}
                            </p>
                          </div>
                          <Button
                            type="button"
                            variant="outline"
                            disabled={pending}
                            onClick={() => setRestoreVersionID(version.id)}
                          >
                            <ArchiveRestore aria-hidden="true" />
                            恢复为草稿
                          </Button>
                        </div>
                      ))}
                    </div>
                  )}
                </section>
              </form>
            ) : null}
          </DialogBody>
          <DialogFooter>
            {configuration.data?.draft ? (
              <Button
                type="button"
                variant="outline"
                disabled={pending}
                onClick={() => setDiscardConfirmOpen(true)}
              >
                <Trash2 aria-hidden="true" />
                删除草稿
              </Button>
            ) : null}
            <Button
              type="submit"
              form="model-check-configuration-form"
              variant="outline"
              disabled={!configuration.data || pending}
            >
              <Save aria-hidden="true" />
              {save.isPending ? "保存中..." : "保存草稿"}
            </Button>
            <Button
              type="button"
              disabled={!configuration.data?.draft || pending}
              onClick={() => setPublishConfirmOpen(true)}
            >
              <Send aria-hidden="true" />
              发布生效
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmActionDialog
        open={publishConfirmOpen}
        title="发布检测画像"
        description="发布后，新创建的模型检测任务将使用该草稿；已经排队或运行中的任务继续使用原画像。"
        confirmLabel="确认发布"
        pendingLabel="发布中..."
        pending={publish.isPending}
        onOpenChange={setPublishConfirmOpen}
        onConfirm={() => publish.mutate()}
      />
      <ConfirmActionDialog
        open={discardConfirmOpen}
        title="删除画像草稿"
        description="未发布的题库和画像修改将被删除，当前已发布版本不会受到影响。"
        confirmLabel="确认删除"
        pendingLabel="删除中..."
        pending={discard.isPending}
        onOpenChange={setDiscardConfirmOpen}
        onConfirm={() => discard.mutate()}
      />
      <ConfirmActionDialog
        open={restoreVersionID !== null}
        title="恢复历史画像"
        description="所选历史版本将复制为新草稿，不会立即影响模型检测。"
        confirmLabel="恢复为草稿"
        pendingLabel="恢复中..."
        pending={restore.isPending}
        onOpenChange={(open) => !open && setRestoreVersionID(null)}
        onConfirm={() => {
          if (restoreVersionID) restore.mutate(restoreVersionID);
        }}
      />
    </>
  );
}

function VersionMetric(props: { label: string; value: string }) {
  return (
    <div className="bg-muted/30 min-w-0 rounded-md border px-3 py-2">
      <p className="text-muted-foreground text-xs">{props.label}</p>
      <Tooltip>
        <TooltipTrigger render={<p className="mt-1 truncate text-sm font-medium" />}>
          {props.value}
        </TooltipTrigger>
        <TooltipContent className="max-w-sm break-words">{props.value}</TooltipContent>
      </Tooltip>
    </div>
  );
}
