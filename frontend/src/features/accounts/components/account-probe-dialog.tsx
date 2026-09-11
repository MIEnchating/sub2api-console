import { QueryErrorToast } from "@/components/query-error-toast";
import { TaskStartupState } from "@/components/task-startup-state";
import { notifyOperationError } from "@/lib/operation-feedback";
import { useOnboardingProbeTask, probeTaskResultSchema } from "../hooks/use-onboarding-probe-task";
import { ProbeTaskTimeline } from "./probe-task-timeline";
import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  Activity,
  CheckCircle2,
  Grid2X2,
  LoaderCircle,
  MessageCircle,
  Play,
  RefreshCw,
  XCircle,
} from "lucide-react";

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
  { value: "stream", label: "Compact 探测" },
];

export const accountProbeDialogLayout = {
  width: "medium",
  height: "adaptive",
  content: "grid grid-rows-[auto_minmax(0,1fr)_auto_auto] overflow-hidden",
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
  const configured = settings?.platform_probe_models?.[canonicalProbePlatform(platform)]?.trim();
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
        <DialogHeader className="min-w-0 border-b px-6 py-5 pr-12">
          <DialogTitle className="min-w-0 break-words">测试账号连接</DialogTitle>
        </DialogHeader>
        <DialogBody className="grid gap-4 px-6 py-4">
          <ProbeAccountCard target={props.target} status={progress.task?.status} />
          <ProbeTaskTimeline steps={progress.history} />
          {(modelsLoading || runProbe.isPending || closing) && progress.history.length === 0 ? (
            <TaskStartupState message="正在创建探活任务" />
          ) : null}
          {closing ? <TaskStartupState message="正在取消探活并清理临时 Key" /> : null}
          {progress.queryError ? (
            <Button variant="outline" onClick={() => void progress.refetch()}>
              重新读取探活状态
            </Button>
          ) : null}
          {(modelsLoading || runProbe.isPending) && !closing ? (
            <Button
              variant="outline"
              onClick={() => cancelProbe.mutate()}
              disabled={cancelProbe.isPending}
            >
              <XCircle aria-hidden="true" />
              取消探活
            </Button>
          ) : null}
          <div className="grid min-w-0 gap-1.5">
            <span className="text-sm font-medium">选择测试模型</span>
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
              <SelectTrigger className="w-full min-w-0" aria-label="选择测试模型">
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
          </div>
          <ProbeModelLoadButton
            pending={modelsLoading}
            succeeded={loadModels.isSuccess}
            disabled={selectDisabled}
            onLoad={startModelLoad}
          />
          {loadModels.isSuccess ? (
            <p className="text-muted-foreground text-xs">已读取 {models.length} 个上游模型。</p>
          ) : null}
          {loadModels.isError ? (
            <QueryErrorToast error={loadModels.error} fallback="上游模型获取失败" />
          ) : null}
          <div className="grid min-w-0 gap-1.5">
            <span className="text-sm font-medium">测试模式</span>
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
          <ProbeResultSlot
            pending={false}
            error={runProbe.isError ? runProbe.error : null}
            result={result}
            requestModel={selectedModel === noModelSelected ? null : selectedModel}
          />
        </DialogBody>
        <div className="text-muted-foreground flex min-w-0 items-center justify-between gap-3 border-t px-6 py-3 text-xs">
          <span className="inline-flex items-center gap-1.5">
            <Grid2X2 className="size-3.5" aria-hidden="true" />
            测试模型
          </span>
          <span className="inline-flex min-w-0 items-center gap-1.5 truncate">
            <MessageCircle className="size-3.5 shrink-0" aria-hidden="true" />
            提示词："hi"
          </span>
        </div>
        <ProbeDialogActions
          runDisabled={runDisabled}
          probePending={runProbe.isPending}
          hasResult={result !== null}
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
      className="w-full min-w-0"
      disabled={props.disabled || props.pending}
      onClick={props.onLoad}
    >
      <RefreshCw className={props.pending ? "animate-spin" : undefined} />
      {label}
    </Button>
  );
}

export function ProbeResultSlot(props: {
  pending: boolean;
  error: Error | null;
  result: ProbeResult | null;
  requestModel?: string | null;
}) {
  if (!props.pending && !props.error && !props.result) return null;

  let content = null;
  if (props.pending) {
    content = (
      <div
        className="max-w-full min-w-0 overflow-hidden rounded-lg border border-zinc-800 bg-zinc-950 px-4 py-3 text-sm text-zinc-100"
        aria-live="polite"
      >
        <div className="grid gap-2 font-mono text-xs">
          <p className="text-cyan-300">使用模型：{props.requestModel ?? "正在确认"}</p>
          <p className="text-zinc-400">发送测试消息："hi"</p>
          <p className="text-amber-300">响应：</p>
          <p className="inline-flex items-center gap-2 text-zinc-400">
            <LoaderCircle className="size-3.5 animate-spin text-cyan-400" aria-hidden="true" />
            等待上游响应
          </p>
        </div>
        <div className="mt-3 flex items-center gap-2 border-t border-zinc-700 pt-3 font-mono text-xs text-cyan-300">
          <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
          测试中
        </div>
      </div>
    );
  } else if (props.error) {
    return <QueryErrorToast error={props.error} fallback="探活请求失败" />;
  } else if (props.result) {
    content = <ProbeResultPanel result={props.result} />;
  }

  return <div className="grid min-h-36 min-w-0 [&>*]:min-h-36">{content}</div>;
}

export function ProbeDialogActions(props: {
  runDisabled: boolean;
  probePending: boolean;
  hasResult: boolean;
  onClose: () => void;
  onRun: () => void;
}) {
  let actionLabel = "开始测试";
  if (props.probePending) actionLabel = "测试中";
  else if (props.hasResult) actionLabel = "重试";
  let actionIcon = <Activity />;
  if (props.probePending) actionIcon = <LoaderCircle className="animate-spin" />;
  else if (props.hasResult) actionIcon = <RefreshCw />;
  return (
    <DialogFooter className="mx-0 mb-0 min-w-0 rounded-none bg-transparent px-6 py-4 sm:flex-row">
      <Button variant="outline" onClick={props.onClose}>
        关闭
      </Button>
      <Button disabled={props.runDisabled} onClick={props.onRun}>
        {actionIcon}
        {actionLabel}
      </Button>
    </DialogFooter>
  );
}

function ProbeResultPanel(props: { result: ProbeResult }) {
  const passed = props.result.status === "passed";
  return (
    <div
      className="max-w-full min-w-0 overflow-hidden rounded-lg border border-zinc-800 bg-zinc-950 px-4 py-3 text-sm text-zinc-100"
      aria-live="polite"
    >
      <div className="grid gap-2 font-mono text-xs">
        <p className="text-cyan-300">使用模型：{props.result.request_model || "未返回"}</p>
        <p className="text-zinc-400">发送测试消息："hi"</p>
        <p className="text-amber-300">响应：</p>
        <p className="min-w-0 whitespace-pre-wrap break-words text-emerald-300 [overflow-wrap:anywhere]">
          {props.result.response_text || props.result.message}
        </p>
        {!passed && props.result.response_text ? (
          <p className="text-red-300 break-words">{props.result.message}</p>
        ) : null}
      </div>
      <div className="mt-3 flex items-center gap-2 border-t border-zinc-700 pt-3 font-mono text-xs">
        {passed ? (
          <CheckCircle2 className="size-4 shrink-0 text-emerald-400" />
        ) : (
          <XCircle className="text-destructive size-4 shrink-0" />
        )}
        <span className={passed ? "text-emerald-400" : "text-destructive"}>
          {passed ? "测试完成！" : "测试失败"}
        </span>
        <span className="ml-auto text-zinc-500">
          HTTP {props.result.http_status > 0 ? props.result.http_status : "-"}
          {props.result.latency_ms > 0 ? ` · ${props.result.latency_ms} 毫秒` : ""}
        </span>
      </div>
    </div>
  );
}

function ProbeAccountCard(props: { target: ProbeDialogTarget; status?: string }) {
  const statusLabels: Record<string, string> = {
    queued: "已排队",
    running: "进行中",
    succeeded: "已完成",
    failed: "失败",
    cancelled: "已取消",
  };
  return (
    <div className="flex min-w-0 items-center gap-3 rounded-lg border bg-muted/20 px-3 py-3">
      <div className="bg-primary flex size-10 shrink-0 items-center justify-center rounded-md text-primary-foreground">
        <Play className="size-5 fill-current" aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-semibold">{props.target.name}</p>
        <div className="text-muted-foreground mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs">
          <span className="rounded bg-muted px-1.5 py-0.5 font-medium uppercase">APIKEY</span>
          <span className="shrink-0">账号</span>
          {props.target.platform ? <span className="truncate">{props.target.platform}</span> : null}
          <span className="truncate">{props.target.host}</span>
        </div>
      </div>
      <span className="shrink-0 rounded-full bg-emerald-100 px-2.5 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300">
        {statusLabels[props.status ?? ""] ?? "待测试"}
      </span>
    </div>
  );
}
