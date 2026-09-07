import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Activity, LoaderCircle } from "lucide-react";
import { useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";

import { api, type AccountStatus, type Task, type TaskSummary } from "@/api";
import { StatusBadge, type StatusVariant } from "@/components/status-badge";
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
import { Input } from "@/components/ui/input";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

import { accountPlatformLabel } from "../lib/account-labels";
import { platformProbeSchema, type PlatformProbeForm } from "../lib/platform-probe-schema";

export type PlatformProbeOption = {
  value: string;
  label: string;
  accountCount: number;
};

export type PlatformProbeResult = {
  accountId: string;
  accountName: string;
  result: string;
  requestModel: string;
  actualModel: string;
  statusCode: number | null;
  failureReason: string;
};

export function platformProbeRequest(values: PlatformProbeForm): {
  platform: string;
  model: string;
} {
  return { platform: values.platform.trim().toLocaleLowerCase(), model: values.model.trim() };
}

export function platformProbeOptions(accounts: AccountStatus[]): PlatformProbeOption[] {
  const counts = new Map<string, number>();
  for (const account of accounts) {
    if (account.manual_priority != null) continue;
    const platform = account.platform?.trim().toLocaleLowerCase();
    if (!platform) continue;
    counts.set(platform, (counts.get(platform) ?? 0) + 1);
  }
  return [...counts.entries()]
    .map(([value, accountCount]) => ({
      value,
      label: accountPlatformLabel(value) ?? value,
      accountCount,
    }))
    .sort((left, right) => left.label.localeCompare(right.label, "zh-CN"));
}

function optionalString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export function platformProbeResults(
  task?: Task,
  accountNames: ReadonlyMap<string, string> = new Map(),
): PlatformProbeResult[] {
  const rawResults = task?.result.results;
  if (!Array.isArray(rawResults)) return [];
  return rawResults.flatMap((raw): PlatformProbeResult[] => {
    if (typeof raw !== "object" || raw === null || Array.isArray(raw)) return [];
    const row = raw as Record<string, unknown>;
    const accountId = optionalString(row.account_id);
    if (!accountId) return [];
    return [
      {
        accountId,
        accountName:
          optionalString(row.account_name) || accountNames.get(accountId) || "账号名称不可用",
        result: optionalString(row.result) || "未知",
        requestModel: optionalString(row.request_model),
        actualModel: optionalString(row.actual_model),
        statusCode: typeof row.status_code === "number" ? row.status_code : null,
        failureReason: optionalString(row.failure_reason),
      },
    ];
  });
}

export function partitionPlatformProbeResults(results: PlatformProbeResult[]): {
  failed: PlatformProbeResult[];
  succeeded: PlatformProbeResult[];
} {
  const failed: PlatformProbeResult[] = [];
  const succeeded: PlatformProbeResult[] = [];
  for (const result of results) {
    if (result.result === "通过") succeeded.push(result);
    else failed.push(result);
  }
  return { failed, succeeded };
}

function taskSummary(task: Task): TaskSummary {
  return {
    id: task.id,
    skill: task.skill,
    operation: task.operation,
    status: task.status,
    progress: task.progress,
    message: task.message,
    created_at: task.created_at,
    updated_at: task.updated_at,
    system_info: true,
  };
}

function probeStatusVariant(result: string): StatusVariant {
  if (result === "通过") return "success";
  if (result === "跳过") return "neutral";
  if (result === "超时") return "warning";
  return "danger";
}

export function PlatformProbeResultTable(props: { results: PlatformProbeResult[] }) {
  const [selectedGroup, setSelectedGroup] = useState<"failed" | "succeeded">("failed");
  const groups = useMemo(() => partitionPlatformProbeResults(props.results), [props.results]);
  const activeGroup =
    selectedGroup === "failed" && groups.failed.length === 0 ? "succeeded" : selectedGroup;
  const visibleResults = groups[activeGroup];

  return (
    <div className="grid min-h-0 gap-2">
      <SegmentedControl role="tablist" aria-label="探活结果分类">
        <SegmentedControlItem
          type="button"
          role="tab"
          selected={activeGroup === "failed"}
          onClick={() => setSelectedGroup("failed")}
        >
          失败与异常 {groups.failed.length}
        </SegmentedControlItem>
        <SegmentedControlItem
          type="button"
          role="tab"
          selected={activeGroup === "succeeded"}
          onClick={() => setSelectedGroup("succeeded")}
        >
          成功 {groups.succeeded.length}
        </SegmentedControlItem>
      </SegmentedControl>
      <Table
        overflowTooltip={false}
        containerClassName="max-h-[min(32rem,calc(100svh-18rem))] overflow-y-auto rounded-md border"
      >
        <TableHeader>
          <TableRow>
            <TableHead className="min-w-40">账号</TableHead>
            <TableHead className="w-24">结果</TableHead>
            <TableHead>请求模型</TableHead>
            <TableHead>实际模型</TableHead>
            <TableHead className="w-20">HTTP</TableHead>
            <TableHead>失败原因</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {visibleResults.map((result) => (
            <TableRow key={result.accountId}>
              <TableCell>
                <div className="font-medium">{result.accountName}</div>
                <div className="text-muted-foreground text-xs">ID {result.accountId}</div>
              </TableCell>
              <TableCell>
                <StatusBadge label={result.result} variant={probeStatusVariant(result.result)} />
              </TableCell>
              <TableCell>{result.requestModel || "-"}</TableCell>
              <TableCell>{result.actualModel || "-"}</TableCell>
              <TableCell>{result.statusCode ?? "-"}</TableCell>
              <TableCell>{result.failureReason || "-"}</TableCell>
            </TableRow>
          ))}
          {visibleResults.length === 0 ? (
            <TableRow>
              <TableCell colSpan={6} className="h-20 text-center text-muted-foreground">
                当前分类暂无结果
              </TableCell>
            </TableRow>
          ) : null}
        </TableBody>
      </Table>
    </div>
  );
}

function FieldError(props: { message?: string }) {
  if (!props.message) return null;
  return (
    <p className="text-destructive text-xs" role="alert">
      {props.message}
    </p>
  );
}

export function PlatformProbeDialog(props: {
  open: boolean;
  accounts: AccountStatus[];
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const options = useMemo(() => platformProbeOptions(props.accounts), [props.accounts]);
  const onlyPlatform = options.length === 1 ? options[0].value : "";
  const form = useForm<PlatformProbeForm>({
    resolver: zodResolver(platformProbeSchema),
    defaultValues: { platform: onlyPlatform, model: "" },
  });
  const selectedPlatform = form.watch("platform");
  const selectedOption = options.find((option) => option.value === selectedPlatform);
  const run = useMutation({
    mutationFn: (values: PlatformProbeForm) => api.runActiveProbe(platformProbeRequest(values)),
    onSuccess: (created) => {
      queryClient.setQueryData(["task", created.id], created);
      queryClient.setQueryData<TaskSummary[]>(["tasks"], (current) =>
        [taskSummary(created), ...(current ?? []).filter((task) => task.id !== created.id)].slice(
          0,
          20,
        ),
      );
      void queryClient.invalidateQueries({ queryKey: ["tasks"] });
      form.reset({ platform: onlyPlatform, model: "" });
      props.onOpenChange(false);
      toast.success("探活任务已转入后台", {
        description: "可在“系统信息”的进行中任务里查看进度和结果。",
      });
    },
  });
  const pending = run.isPending;

  function changeOpen(open: boolean) {
    if (open) {
      props.onOpenChange(true);
      return;
    }
    run.reset();
    form.reset({ platform: onlyPlatform, model: "" });
    props.onOpenChange(false);
  }

  const submit = form.handleSubmit((values) => run.mutate(values));

  return (
    <Dialog open={props.open} onOpenChange={changeOpen}>
      <DialogContent width="wide" height="adaptive">
        <DialogHeader>
          <DialogTitle>平台模型探活</DialogTitle>
          <DialogDescription>对所选平台下每个可探活账号真实请求一次输入模型。</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="contents">
          <DialogBody className="grid gap-4 py-1">
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="grid gap-1.5 text-sm font-medium">
                平台
                <Select
                  value={selectedPlatform || null}
                  disabled={pending || options.length === 0}
                  onValueChange={(value) =>
                    form.setValue("platform", value ?? "", { shouldValidate: true })
                  }
                >
                  <SelectTrigger
                    aria-label="选择探活平台"
                    aria-invalid={Boolean(form.formState.errors.platform)}
                  >
                    <SelectValue placeholder="选择平台" />
                  </SelectTrigger>
                  <SelectContent align="start">
                    {options.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}（{option.accountCount} 个账号）
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FieldError message={form.formState.errors.platform?.message} />
              </label>
              <label className="grid gap-1.5 text-sm font-medium">
                探活模型
                <Input
                  aria-label="输入探活模型"
                  aria-invalid={Boolean(form.formState.errors.model)}
                  autoComplete="off"
                  disabled={pending}
                  placeholder="例如 gpt-5.6-sol"
                  {...form.register("model")}
                />
                <FieldError message={form.formState.errors.model?.message} />
              </label>
            </div>

            {options.length === 0 ? (
              <p className="text-muted-foreground text-sm">当前没有可探活且带平台标识的账号。</p>
            ) : null}
            {options.length > 0 && selectedOption ? (
              <p className="text-muted-foreground text-sm">
                本次将探活 {selectedOption.label} 下的 {selectedOption.accountCount} 个账号。
              </p>
            ) : null}

            {run.error ? (
              <p className="text-destructive text-sm" role="alert">
                {run.error instanceof Error ? run.error.message : "探活任务启动失败"}
              </p>
            ) : null}
            <p className="text-muted-foreground text-xs">
              任务启动后弹窗会自动关闭，可在“系统信息”中查看进度和结果。
            </p>
          </DialogBody>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => changeOpen(false)}>
              关闭
            </Button>
            <Button type="submit" disabled={pending || options.length === 0}>
              {pending ? (
                <LoaderCircle className="animate-spin" aria-hidden="true" />
              ) : (
                <Activity aria-hidden="true" />
              )}
              {pending ? "正在启动" : "开始探活"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
