import { useEffect, useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Activity, CheckCircle2, LoaderCircle, RefreshCw, XCircle } from "lucide-react";

import { api, type ProbeResult } from "@/api";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { operationErrorMessage } from "@/lib/operation-feedback";

const noModelSelected = "__not_selected__";

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
};

export function onboardingProbeModelOptions(models: string[]): string[] {
  return [...new Set(models)].sort((left, right) => left.localeCompare(right));
}

export function shouldLoadProbeModels(
  open: boolean,
  modelCount: number,
  pending: boolean,
  succeeded: boolean,
): boolean {
  return open && modelCount === 0 && !pending && !succeeded;
}

export function AccountProbeDialog(props: {
  target: ProbeDialogTarget;
  open: boolean;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onCompleted?: () => void;
}) {
  const [models, setModels] = useState<string[]>([]);
  const [selectedModel, setSelectedModel] = useState(noModelSelected);
  const [result, setResult] = useState<ProbeResult | null>(null);
  const loadModels = useMutation({
    mutationFn: () => api.onboardingProbeModels(props.target.host, props.target.groupId),
    onSuccess: (response) => {
      setModels(response.models);
      if (selectedModel === noModelSelected && response.models.length > 0) {
        setSelectedModel(response.models[0]);
      }
    },
  });
  const runProbe = useMutation({
    mutationFn: async () => {
      if (selectedModel === noModelSelected) throw new Error("请先获取并选择一个上游模型");
      return api.runOnboardingProbe(props.target.host, props.target.groupId, selectedModel);
    },
    onMutate: () => setResult(null),
    onSuccess: (probeResult) => {
      setResult(probeResult);
      props.onCompleted?.();
    },
  });

  useEffect(() => {
    if (!props.open) {
      setModels([]);
      setResult(null);
      setSelectedModel(noModelSelected);
      loadModels.reset();
      runProbe.reset();
    }
  }, [props.open]);

  const options = useMemo(() => onboardingProbeModelOptions(models), [models]);
  const selectDisabled = Boolean(props.pending) || runProbe.isPending;
  const runDisabled = selectDisabled || loadModels.isPending || selectedModel === noModelSelected;

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        width={accountProbeDialogLayout.width}
        height={accountProbeDialogLayout.height}
        className={accountProbeDialogLayout.content}
      >
        <DialogHeader className="min-w-0 pr-8">
          <DialogTitle className="min-w-0 break-words">探活测试：{props.target.name}</DialogTitle>
          <DialogDescription className="min-w-0 break-words">
            添加账号前读取该上游支持的模型并执行单次测试，结果会保留在此弹窗中。
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid gap-3 py-1">
          <div className="grid min-w-0 gap-1.5">
            <span className="text-sm font-medium">测试模型</span>
            <Select
              value={selectedModel}
              itemToStringLabel={(value) =>
                value === noModelSelected ? "选择上游模型" : String(value)
              }
              disabled={selectDisabled}
              onOpenChange={(open) => {
                if (
                  shouldLoadProbeModels(
                    open,
                    models.length,
                    loadModels.isPending,
                    loadModels.isSuccess,
                  )
                ) {
                  loadModels.mutate();
                }
              }}
              onValueChange={(value) => {
                if (!value) return;
                setSelectedModel(value);
                setResult(null);
                runProbe.reset();
              }}
            >
              <SelectTrigger className="w-full min-w-0">
                <SelectValue placeholder="选择上游模型" />
              </SelectTrigger>
              <SelectContent>
                {loadModels.isPending ? (
                  <SelectItem value={noModelSelected} disabled>
                    正在获取上游模型
                  </SelectItem>
                ) : null}
                {!loadModels.isPending && options.length === 0 ? (
                  <SelectItem value={noModelSelected} disabled>
                    {loadModels.isError ? "获取失败，重新打开可重试" : "暂无可用模型"}
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
            pending={loadModels.isPending}
            succeeded={loadModels.isSuccess}
            disabled={selectDisabled}
            onLoad={() => loadModels.mutate()}
          />
          {loadModels.isSuccess ? (
            <p className="text-muted-foreground text-xs">已读取 {models.length} 个上游模型。</p>
          ) : null}
          {loadModels.isError ? (
            <p
              className="text-destructive min-w-0 break-words text-sm [overflow-wrap:anywhere]"
              role="alert"
            >
              {operationErrorMessage(loadModels.error, "上游模型获取失败")}
            </p>
          ) : null}
          <ProbeResultSlot
            pending={runProbe.isPending}
            error={runProbe.isError ? runProbe.error : null}
            result={result}
          />
        </DialogBody>
        <ProbeDialogActions
          runDisabled={runDisabled}
          probePending={runProbe.isPending}
          hasResult={result !== null}
          onClose={() => props.onOpenChange(false)}
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
}) {
  if (!props.pending && !props.error && !props.result) return null;

  let content = null;
  if (props.pending) {
    content = (
      <div
        className="bg-muted/40 flex min-w-0 items-center gap-3 rounded-lg border px-4 py-3"
        aria-live="polite"
      >
        <LoaderCircle className="text-primary size-5 shrink-0 animate-spin" />
        <div className="min-w-0">
          <p className="text-sm font-medium">正在测试上游响应</p>
          <p className="text-muted-foreground text-xs">弹窗会在测试完成后显示详细结果。</p>
        </div>
      </div>
    );
  } else if (props.error) {
    content = (
      <div
        className="border-destructive/40 bg-destructive/5 min-w-0 overflow-hidden rounded-lg border px-4 py-3"
        role="alert"
      >
        <div className="text-destructive flex items-center gap-2 text-sm font-medium">
          <XCircle className="size-4 shrink-0" />
          探活失败
        </div>
        <p className="text-muted-foreground mt-1.5 min-w-0 break-words text-sm [overflow-wrap:anywhere]">
          {operationErrorMessage(props.error, "探活请求失败")}
        </p>
      </div>
    );
  } else if (props.result) {
    content = <ProbeResultPanel result={props.result} />;
  }

  return <div className="grid min-h-36 min-w-0">{content}</div>;
}

export function ProbeDialogActions(props: {
  runDisabled: boolean;
  probePending: boolean;
  hasResult: boolean;
  onClose: () => void;
  onRun: () => void;
}) {
  return (
    <DialogFooter className="min-w-0">
      <Button variant="outline" onClick={props.onClose}>
        关闭
      </Button>
      <Button disabled={props.runDisabled} onClick={props.onRun}>
        {props.probePending ? <LoaderCircle className="animate-spin" /> : <Activity />}
        {props.probePending ? "测试中" : props.hasResult ? "再次探活" : "开始探活"}
      </Button>
    </DialogFooter>
  );
}

function ProbeResultPanel(props: { result: ProbeResult }) {
  const passed = props.result.status === "passed";
  return (
    <div
      className={
        passed
          ? "border-emerald-500/40 bg-emerald-500/5 max-w-full min-w-0 overflow-hidden rounded-lg border px-4 py-3"
          : "border-destructive/40 bg-destructive/5 max-w-full min-w-0 overflow-hidden rounded-lg border px-4 py-3"
      }
      aria-live="polite"
    >
      <div
        className={
          passed
            ? "flex items-center gap-2 text-sm font-semibold text-emerald-600 dark:text-emerald-400"
            : "text-destructive flex items-center gap-2 text-sm font-semibold"
        }
      >
        {passed ? (
          <CheckCircle2 className="size-4 shrink-0" />
        ) : (
          <XCircle className="size-4 shrink-0" />
        )}
        {passed ? "探活通过" : "探活失败"}
      </div>
      <p className="mt-1.5 min-w-0 break-words text-sm [overflow-wrap:anywhere]">
        {props.result.message}
      </p>
      <dl className="mt-3 grid min-w-0 grid-cols-2 gap-x-4 gap-y-2 text-xs sm:grid-cols-4">
        <ResultValue label="请求模型" value={props.result.request_model || "未返回"} />
        <ResultValue label="实际模型" value={props.result.actual_model || "未返回"} />
        <ResultValue
          label="HTTP 状态"
          value={props.result.http_status > 0 ? String(props.result.http_status) : "未返回"}
        />
        <ResultValue
          label="耗时"
          value={props.result.latency_ms > 0 ? `${props.result.latency_ms} 毫秒` : "未记录"}
        />
      </dl>
    </div>
  );
}

function ResultValue(props: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground">{props.label}</dt>
      <Tooltip>
        <TooltipTrigger render={<dd className="mt-0.5 truncate font-medium" />}>
          {props.value}
        </TooltipTrigger>
        <TooltipContent className="max-w-xs break-all">{props.value}</TooltipContent>
      </Tooltip>
    </div>
  );
}
