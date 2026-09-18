import { QueryErrorToast } from "@/components/query-error-toast";
import { notifyOperationError } from "@/lib/operation-feedback";
import { useOnboardingProbeTask, probeTaskResultSchema } from "../hooks/use-onboarding-probe-task";
import { ProbeProgressSummary } from "./probe-task-timeline";
import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Activity, CheckCircle2, LoaderCircle, Play, RefreshCw, XCircle } from "lucide-react";

import {
  api,
  type AccountCreationSettings,
  type OnboardingProbeMode,
  type ProbeResult,
} from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const noModelSelected = "__not_selected__";

export const onboardingProbeModeOptions: Array<{
  value: OnboardingProbeMode;
  label: string;
}> = [
  { value: "default", label: "常规请求" },
  { value: "stream", label: "流式请求" },
];

export const accountProbeDialogLayout = {
  width: "medium",
  height: "adaptive",
  content: "grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden",
} as const;

export type ProbeDialogTarget = {
  kind: "onboarding";
  host: string;
  groupId: string;
  name: string;
  platform?: string | null;
};

export function onboardingProbeModelOptions(models: string[]): string[] {
  return [...new Set(models)].sort((left, right) => left.localeCompare(right));
}

function canonicalProbePlatform(platform: string | null | undefined): string {
  const normalized = platform?.trim().toLocaleLowerCase() ?? "";
  if (normalized === "claude") return "anthropic";
  if (normalized === "google") return "gemini";
  return normalized;
}

export function defaultProbeModelForPlatform(
  models: string[],
  settings: AccountCreationSettings | undefined,
  platform: string | null | undefined,
): string | null {
  const options = onboardingProbeModelOptions(models);
  if (options.length === 0) return null;
  const platformModels = settings?.platform_probe_models;
  const platformKey = canonicalProbePlatform(platform);
  const configured =
    platformModels && Object.hasOwn(platformModels, platformKey)
      ? platformModels[platformKey]?.trim()
      : undefined;
  if (!configured) return options[0] ?? null;
  return (
    options.find((model) => model.toLocaleLowerCase() === configured.toLocaleLowerCase()) ??
    options[0] ??
    null
  );
}

export function shouldLoadProbeModels(
  open: boolean,
  loadedTargetKey: string | null,
  targetKey: string,
): boolean {
  return open && loadedTargetKey !== targetKey;
}

export function AccountProbeDialog(props: {
  target: ProbeDialogTarget;
  open: boolean;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onCompleted?: () => void;
}) {
  if (!props.open) return null;
  return <AccountProbeSession key={`${props.target.host}:${props.target.groupId}`} {...props} />;
}

function AccountProbeSession(props: {
  target: ProbeDialogTarget;
  open: boolean;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onCompleted?: () => void;
}) {
  const [models, setModels] = useState<string[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [selectedModel, setSelectedModel] = useState(noModelSelected);
  const [selectedMode, setSelectedMode] = useState<OnboardingProbeMode>("default");
  const [result, setResult] = useState<ProbeResult | null>(null);
  const progress = useOnboardingProbeTask(props.target.host, props.target.groupId);
  const [closing, setClosing] = useState(false);
  const accountSettings = useQuery({
    queryKey: ["account-creation-settings"],
    queryFn: api.accountCreationSettings,
    enabled: props.open,
    staleTime: 60_000,
  });
  const loadedTargetKey = useRef<string | null>(null);
  const modelLoadInFlight = useRef(false);
  const modelLoadStarted = useRef(false);
  const modelSelectionEdited = useRef(false);
  const loadModels = useMutation({
    mutationFn: async () => {
      const task = await progress.run("models");
      if (task.status !== "succeeded") throw new Error(task.message);
      const values = task.result.models;
      if (
        !Array.isArray(values) ||
        !values.every((value): value is string => typeof value === "string")
      )
        throw new Error("模型列表格式无效，请重新获取");
      return { models: values };
    },
    onSuccess: (response) => {
      setModels(response.models);
      setSelectedModel((current) => {
        if (current !== noModelSelected && response.models.includes(current)) return current;
        return (
          defaultProbeModelForPlatform(
            response.models,
            accountSettings.data,
            props.target.platform,
          ) ?? noModelSelected
        );
      });
    },
    onSettled: () => {
      modelLoadInFlight.current = false;
      setModelsLoading(false);
    },
  });
  const runProbe = useMutation({
    mutationFn: async () => {
      if (selectedModel === noModelSelected) throw new Error("请先获取并选择一个上游模型");
      const task = await progress.run("probe", selectedModel, selectedMode);
      const parsed = probeTaskResultSchema.safeParse(task.result.probe_result);
      if (!parsed.success) throw new Error(task.message);
      if (task.status !== "succeeded")
        return { ...parsed.data, status: "failed" as const, message: task.message };
      return parsed.data;
    },
    onMutate: () => {
      setResult(null);
    },
    onSuccess: (probeResult) => {
      setResult(probeResult);
      props.onCompleted?.();
    },
  });
  const cancelProbe = useMutation({
    mutationFn: async () => {
      await progress.cancel();
      const task = await progress.run("cleanup");
      if (task.status !== "succeeded") throw new Error(task.message);
    },
    onError: (error) => notifyOperationError(error, "临时 Key 清理失败"),
  });
  function startModelLoad() {
    if (modelLoadInFlight.current) return;
    modelLoadInFlight.current = true;
    setModelsLoading(true);
    loadModels.mutate();
  }

  useEffect(() => {
    if (!props.open) {
      loadedTargetKey.current = null;
      modelLoadInFlight.current = false;
      modelLoadStarted.current = false;
      modelSelectionEdited.current = false;
      setModels([]);
      setModelsLoading(false);
      setResult(null);
      setSelectedModel(noModelSelected);
      setSelectedMode("default");
      loadModels.reset();
      runProbe.reset();
      cancelProbe.reset();
      return;
    }
    const targetKey = `${props.target.host}\u0000${props.target.groupId}`;
    if (!shouldLoadProbeModels(props.open, loadedTargetKey.current, targetKey)) return;
    loadedTargetKey.current = targetKey;
    modelLoadStarted.current = true;
    modelSelectionEdited.current = false;
    setModels([]);
    setResult(null);
    setSelectedModel(noModelSelected);
    setSelectedMode("default");
    loadModels.reset();
    runProbe.reset();
    startModelLoad();
  }, [props.open, props.target.host, props.target.groupId]);

  useEffect(() => {
    if (!props.open || modelSelectionEdited.current || models.length === 0) return;
    const configured = defaultProbeModelForPlatform(
      models,
      accountSettings.data,
      props.target.platform,
    );
    if (configured) setSelectedModel(configured);
  }, [accountSettings.data, models, props.open, props.target.platform]);

  const options = useMemo(() => onboardingProbeModelOptions(models), [models]);
  const selectDisabled =
    Boolean(props.pending) || runProbe.isPending || closing || cancelProbe.isPending;
  const runDisabled = selectDisabled || modelsLoading || selectedModel === noModelSelected;
  let pendingMessage: string | undefined;
  if (closing || cancelProbe.isPending) pendingMessage = "正在取消探活并清理临时 Key";
  else if (
    (modelsLoading || runProbe.isPending) &&
    (progress.starting || progress.history.length === 0)
  )
    pendingMessage = "正在创建探活任务";
  async function closeProbe() {
    if (closing) return;
    setClosing(true);
    try {
      if (modelLoadStarted.current) await cancelProbe.mutateAsync();
      props.onOpenChange(false);
    } catch {
      // Keep the timeline visible so cleanup can be retried.
    } finally {
      setClosing(false);
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={(open) => !open && closeProbe()}>
      <DialogContent
        width={accountProbeDialogLayout.width}
        height={accountProbeDialogLayout.height}
        className={`${accountProbeDialogLayout.content} gap-0 p-0`}
      >
        <DialogHeader className="min-w-0 border-b px-4 py-4 pr-12 sm:px-5">
          <DialogTitle className="min-w-0 break-words">测试账号连接</DialogTitle>
        </DialogHeader>
        <DialogBody className="grid min-w-0 content-start gap-3 px-4 py-3 sm:px-5 [scrollbar-gutter:stable]">
          <ProbeAccountCard target={props.target} />
          <div className="grid min-w-0 grid-cols-1 gap-3" role="group" aria-label="探活参数">
            <div className="grid min-w-0 gap-1.5">
              <span className="text-sm font-medium">模型</span>
              <div
                role="group"
                aria-label="测试模型选择与获取"
                className="flex min-w-0 items-center gap-1.5"
              >
                <Select
                  value={selectedModel}
                  itemToStringLabel={(value) =>
                    value === noModelSelected ? "选择上游模型" : String(value)
                  }
                  disabled={selectDisabled}
                  onValueChange={(value) => {
                    if (!value) return;
                    modelSelectionEdited.current = true;
                    setSelectedModel(value);
                    setResult(null);
                    runProbe.reset();
                  }}
                >
                  <SelectTrigger className="min-w-0 flex-1" aria-label="选择测试模型">
                    <SelectValue placeholder="选择上游模型" />
                  </SelectTrigger>
                  <SelectContent>
                    {modelsLoading ? (
                      <SelectItem value={noModelSelected} disabled>
                        正在获取上游模型
                      </SelectItem>
                    ) : null}
                    {!modelsLoading && options.length === 0 ? (
                      <SelectItem value={noModelSelected} disabled>
                        {loadModels.isError ? "—" : "暂无可用模型"}
                      </SelectItem>
                    ) : null}
                    {options.map((model) => (
                      <SelectItem key={model} value={model}>
                        {model}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <ProbeModelLoadButton
                  pending={modelsLoading}
                  succeeded={loadModels.isSuccess}
                  disabled={selectDisabled}
                  onLoad={startModelLoad}
                />
              </div>
            </div>
            {loadModels.isError ? (
              <QueryErrorToast error={loadModels.error} fallback="上游模型获取失败" />
            ) : null}
            <div className="grid min-w-0 gap-1.5">
              <span className="text-sm font-medium">模式</span>
              <Select
                value={selectedMode}
                itemToStringLabel={(value) =>
                  onboardingProbeModeOptions.find((option) => option.value === value)?.label ??
                  String(value)
                }
                disabled={selectDisabled}
                onValueChange={(value) => {
                  if (value !== "stream" && value !== "default") return;
                  setSelectedMode(value);
                  setResult(null);
                  runProbe.reset();
                }}
              >
                <SelectTrigger className="w-full min-w-0" aria-label="选择测试模式">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {onboardingProbeModeOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <section aria-label="测试模型响应" className="grid min-w-0 gap-2">
            <div role="region" aria-label="探活进度" className="min-w-0">
              <ProbeProgressSummary steps={progress.history} pendingMessage={pendingMessage}>
                <ProbeResultSlot
                  pending={runProbe.isPending}
                  pendingLabel="当前阶段见上方"
                  error={null}
                  result={result}
                  requestModel={selectedModel === noModelSelected ? null : selectedModel}
                />
              </ProbeProgressSummary>
              {runProbe.isError ? (
                <QueryErrorToast error={runProbe.error} fallback="探活请求失败" />
              ) : null}
              {progress.queryError ? (
                <Button variant="outline" onClick={() => void progress.refetch()}>
                  重新读取探活状态
                </Button>
              ) : null}
            </div>
            <p className="text-muted-foreground text-xs">使用已配置的探活提示词</p>
          </section>
        </DialogBody>
        <ProbeDialogActions
          runDisabled={runDisabled}
          probePending={runProbe.isPending}
          modelsPending={modelsLoading}
          hasResult={result !== null}
          closing={closing}
          onClose={closeProbe}
          onRun={() => runProbe.mutate()}
        />
      </DialogContent>
    </Dialog>
  );
}

export function ProbeModelLoadButton(props: {
  pending: boolean;
  succeeded: boolean;
  disabled: boolean;
  onLoad: () => void;
}) {
  let label = "获取上游模型";
  if (props.pending) label = "正在获取";
  else if (props.succeeded) label = "重新获取上游模型";
  return (
    <Button
      type="button"
      variant="outline"
      size="icon"
      className="shrink-0 overflow-hidden"
      aria-label={label}
      disabled={props.disabled || props.pending}
      onClick={props.onLoad}
    >
      <RefreshCw className={props.pending ? "animate-spin" : undefined} aria-hidden="true" />
    </Button>
  );
}

export function ProbeResultSlot(props: {
  pending: boolean;
  error: Error | null;
  result: ProbeResult | null;
  requestModel?: string | null;
  pendingLabel?: string;
}) {
  const result = props.pending ? null : props.result;
  const passed = result?.status === "passed";
  let status = "待开始";
  let tone = "text-zinc-400";
  if (props.pending) {
    status = "测试中";
    tone = "text-cyan-300";
  } else if (result) {
    status = passed ? "测试完成！" : "测试失败";
    tone = passed ? "text-emerald-300" : "text-red-300";
  } else if (props.error) status = "未完成";
  return (
    <div
      className="flex h-full min-h-36 min-w-0 flex-col gap-3 overflow-hidden bg-zinc-950 p-3 font-mono text-xs text-zinc-300"
      aria-live="polite"
    >
      {props.error ? <QueryErrorToast error={props.error} fallback="探活请求失败" /> : null}
      <div className="grid shrink-0 gap-1">
        <p className="line-clamp-2 break-words text-cyan-300 [overflow-wrap:anywhere]">
          使用模型：{result?.request_model || props.requestModel || "待选择"}
        </p>
        {result?.actual_model && result.actual_model !== result.request_model ? (
          <p className="line-clamp-2 break-words text-zinc-400 [overflow-wrap:anywhere]">
            响应模型：{result.actual_model}
          </p>
        ) : null}
      </div>
      <div
        role="region"
        aria-label="探活响应内容"
        tabIndex={0}
        className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain whitespace-pre-wrap break-words outline-offset-2 [overflow-wrap:anywhere] [scrollbar-gutter:stable]"
      >
        {result ? (
          <>
            <p className="mb-2 text-zinc-500">响应</p>
            <p className={passed ? "text-emerald-300" : "text-red-300"}>
              {result.response_text || result.message}
            </p>
            {!passed && result.response_text ? (
              <p className="mt-2 text-red-300">{result.message}</p>
            ) : null}
          </>
        ) : null}
        {props.pending ? (
          <div className="grid gap-2 py-2">
            <p>正在发送探活请求</p>
            <p className="text-zinc-500">{props.pendingLabel ?? "等待上游响应"}</p>
          </div>
        ) : null}
        {!props.pending && !result ? (
          <p className="py-2 text-zinc-400">
            {props.error ? "响应尚未返回" : "点击“开始测试”查看响应"}
          </p>
        ) : null}
      </div>
      <div
        className={`flex min-h-5 shrink-0 flex-wrap items-center gap-2 border-t border-zinc-800 pt-2 ${tone}`}
      >
        {props.pending ? (
          <span className="flex size-4 shrink-0 overflow-hidden">
            <LoaderCircle
              aria-hidden="true"
              className="size-4 animate-spin motion-reduce:animate-none"
            />
          </span>
        ) : null}
        {result ? (
          <>
            {passed ? (
              <CheckCircle2 aria-hidden="true" className="size-4 shrink-0" />
            ) : (
              <XCircle aria-hidden="true" className="size-4 shrink-0" />
            )}
          </>
        ) : null}
        <span>{status}</span>
        {result ? (
          <span className="ml-auto text-zinc-400">
            HTTP {result.http_status > 0 ? result.http_status : "-"}
            {result.latency_ms > 0 ? ` · ${result.latency_ms} 毫秒` : ""}
          </span>
        ) : null}
      </div>
    </div>
  );
}

export function ProbeDialogActions(props: {
  runDisabled: boolean;
  probePending: boolean;
  modelsPending?: boolean;
  hasResult: boolean;
  closing?: boolean;
  onClose: () => void;
  onRun: () => void;
}) {
  let actionLabel = "开始测试";
  if (props.probePending) actionLabel = "测试中";
  else if (props.hasResult) actionLabel = "重试";
  let actionIcon = <Activity />;
  if (props.probePending)
    actionIcon = (
      <span className="flex size-4 shrink-0 overflow-hidden">
        <LoaderCircle
          aria-hidden="true"
          className="size-4 animate-spin motion-reduce:animate-none"
        />
      </span>
    );
  else if (props.hasResult) actionIcon = <RefreshCw />;
  let closeLabel = "关闭";
  if (props.closing) closeLabel = "正在关闭";
  else if (props.probePending || props.modelsPending) closeLabel = "取消并关闭";
  return (
    <DialogFooter
      role="group"
      aria-label="探活操作"
      className="mx-0 mb-0 min-w-0 flex-row flex-wrap justify-end gap-2 rounded-none border-t bg-transparent px-4 py-3 sm:px-5"
    >
      <Button
        variant="outline"
        onClick={props.onClose}
        aria-busy={props.closing}
        disabled={props.closing}
      >
        {closeLabel}
      </Button>
      <Button disabled={props.runDisabled} onClick={props.onRun}>
        {actionIcon}
        {actionLabel}
      </Button>
    </DialogFooter>
  );
}

function ProbeAccountCard(props: { target: ProbeDialogTarget }) {
  return (
    <div className="flex min-w-0 items-center gap-2.5">
      <div className="bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md">
        <Play className="size-4" aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{props.target.name}</p>
        <p className="text-muted-foreground truncate text-xs">{props.target.host}</p>
      </div>
      {props.target.platform ? (
        <span className="text-muted-foreground max-w-24 truncate rounded-md border px-2 py-1 text-xs">
          {props.target.platform}
        </span>
      ) : null}
    </div>
  );
}
