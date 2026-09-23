import { useEffect, useMemo, useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { ListChecks, X } from "lucide-react";
import { useForm, useWatch } from "react-hook-form";
import { api } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentRetry } from "@/components/content-retry";
import { DataTablePagination } from "@/components/data-table/pagination";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { TaskCancelButton } from "@/components/task-startup-state";
import { animationSchema, type AnimationForm } from "../lib/animation-schema";
import { useTerminalContinuity } from "../hooks/use-terminal-continuity";
import { AnimationAccountsSkeleton } from "./animation-accounts-skeleton";
import { AnimationModelSettings } from "./animation-model-settings";
import { TerminalContinuityCard } from "./terminal-continuity-card";
import { toast } from "sonner";
import { Input } from "@/components/ui/input";
import { FieldError } from "@/components/field-error";
import { terminalRoundsSchema } from "../lib/detection-task-schema";
import { AnimationAccountFilters } from "./animation-account-filters";
import {
  defaultAnimationFilters,
  filterAnimationAccounts,
  selectableDetectionAccountIDs,
  type AnimationFilters,
} from "../lib/animation-filters";

const pageSizes = [6, 12, 24];

export function TerminalContinuityPanel(props: {
  accountID?: string;
  filters?: AnimationFilters;
  onFiltersChange?: (value: AnimationFilters) => void;
}): ReactElement {
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const continuity = useTerminalContinuity();
  const roundsForm = useForm<{ rounds: number }>({
    resolver: zodResolver(terminalRoundsSchema),
    defaultValues: { rounds: 1 },
  });
  const [localFilters, setLocalFilters] = useState(defaultAnimationFilters);
  const filters = props.filters ?? localFilters;
  const setFilters = props.onFiltersChange ?? setLocalFilters;
  const [confirmation, setConfirmation] = useState<(AnimationForm & { rounds: number }) | null>(
    null,
  );
  const form = useForm<AnimationForm>({
    resolver: zodResolver(animationSchema),
    defaultValues: { account_ids: [], unified_model: "", timeout_seconds: 120 },
  });
  const selected = useWatch({ control: form.control, name: "account_ids", exact: true });
  const rows = useMemo(
    () =>
      filterAnimationAccounts(
        (accounts.data ?? []).filter(
          (account) => !props.accountID || account.id === props.accountID,
        ),
        filters,
      ),
    [accounts.data, filters, props.accountID],
  );
  const pagination = useClientPagination(rows, 12);
  useEffect(() => {
    pagination.setCurrentPage(1);
  }, [filters, props.accountID, pagination.setCurrentPage]);
  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const availableIDs = new Set(
    selectableDetectionAccountIDs(accounts.data ?? [], continuity.busyIDs),
  );
  const availableSelected = selected.filter((id) => availableIDs.has(id));
  const selectableIDs = selectableDetectionAccountIDs(rows, continuity.busyIDs);
  const suspectedIDs = rows
    .filter(
      (account) =>
        continuity.results.get(account.id)?.verdict === "suspected" && availableIDs.has(account.id),
    )
    .map((account) => account.id);
  const toggle = (id: string, checked: boolean): void => {
    const current = form.getValues("account_ids");
    form.setValue(
      "account_ids",
      checked ? [...new Set([...current, id])] : current.filter((value) => value !== id),
      { shouldValidate: true },
    );
  };
  useEffect(() => {
    if (!accounts.data) return;
    const existing = new Set(accounts.data.map((account) => account.id));
    const current = form.getValues("account_ids");
    const next = current.filter((id) => existing.has(id));
    if (next.length !== current.length)
      form.setValue("account_ids", next, { shouldValidate: true });
  }, [accounts.data, form]);
  return (
    <form
      noValidate
      className="flex h-full min-h-0 flex-col overflow-y-auto md:overflow-hidden"
      onSubmit={form.handleSubmit(
        (value) =>
          void roundsForm.handleSubmit((settings) =>
            setConfirmation({
              ...value,
              rounds: settings.rounds,
              account_ids: value.account_ids.filter((id) => availableIDs.has(id)),
            }),
          )(),
        (errors) => {
          if (errors.account_ids?.message) toast.error(errors.account_ids.message);
        },
      )}
    >
      <div role="group" aria-label="终端续接检测设置" className="shrink-0 border-b">
        <div className="flex min-w-0 flex-wrap items-start gap-3 px-3 py-3 sm:px-4">
          <div className="min-w-0 flex-1 basis-[28rem] space-y-1.5">
            <AnimationAccountFilters
              accounts={accounts.data ?? []}
              value={filters}
              onChange={(value) => {
                setFilters(value);
                pagination.setCurrentPage(1);
              }}
              searchLabel="搜索终端续接检测账号"
            />
            <p className="text-xs text-muted-foreground">
              结果仅反映模型能否延续已声明的工具上下文，不修改健康分、熔断或调度。
            </p>
          </div>
          <AnimationModelSettings form={form} pending={continuity.run.isPending} />
          <div className="space-y-1">
            <label className="block space-y-1 text-sm">
              终端检测轮数
              <Input
                type="number"
                min={1}
                max={20}
                {...roundsForm.register("rounds", { valueAsNumber: true })}
                disabled={continuity.run.isPending}
                aria-invalid={!!roundsForm.formState.errors.rounds}
              />
            </label>
            <FieldError message={roundsForm.formState.errors.rounds?.message} />
            <p className="text-xs text-muted-foreground">每轮独立请求，保留逐轮结果。</p>
          </div>
        </div>
        <div
          role="group"
          aria-label="终端续接检测操作"
          className="flex min-w-0 flex-wrap items-center gap-2 border-t bg-muted/20 px-3 py-2 sm:px-4"
        >
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label="全选账号"
                  disabled={
                    continuity.run.isPending || !accounts.isSuccess || selectableIDs.length === 0
                  }
                  onClick={() =>
                    form.setValue("account_ids", selectableIDs, { shouldValidate: true })
                  }
                />
              }
            >
              <ListChecks aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent>全选账号</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label="清空终端续接选择"
                  onClick={() => form.setValue("account_ids", [], { shouldValidate: true })}
                />
              }
            >
              <X aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent>清空选择</TooltipContent>
          </Tooltip>
          <Button
            type="button"
            variant="outline"
            disabled={continuity.run.isPending || suspectedIDs.length === 0}
            onClick={() => form.setValue("account_ids", suspectedIDs, { shouldValidate: true })}
          >
            选择疑似异常（{suspectedIDs.length}）
          </Button>
          <div className="ml-auto flex items-center gap-2">
            {continuity.activeTask ? (
              <TaskCancelButton taskId={continuity.activeTask.id} compact />
            ) : null}
            <Button
              type="submit"
              className="w-40 tabular-nums"
              disabled={continuity.run.isPending || availableSelected.length === 0}
            >
              {continuity.run.isPending
                ? "正在启动检测"
                : `开始检测（${availableSelected.length} 个账号）`}
            </Button>
          </div>
        </div>
      </div>
      <div
        role="region"
        aria-label="终端续接检测账号"
        className="min-h-80 flex-none overflow-visible bg-muted/10 p-3 sm:p-4 md:min-h-0 md:flex-1 md:overflow-y-auto"
      >
        {accounts.isLoading ? <AnimationAccountsSkeleton /> : null}
        {accounts.isError && !accounts.data ? (
          <ContentRetry onRetry={() => void accounts.refetch()} pending={accounts.isFetching} />
        ) : null}
        {continuity.history.isError || continuity.taskError ? (
          <ContentRetry
            onRetry={() => {
              if (continuity.history.isError) void continuity.history.refetch();
              continuity.retryTasks();
            }}
            pending={continuity.history.isFetching || continuity.taskFetching}
          />
        ) : null}
        {accounts.isSuccess && rows.length === 0 ? (
          <p className="text-sm text-muted-foreground">没有匹配的账号</p>
        ) : null}
        <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,20rem),1fr))] items-start gap-3">
          {pagination.visibleItems.map((account) => (
            <TerminalContinuityCard
              key={account.id}
              account={account}
              result={continuity.results.get(account.id)}
              checked={selectedSet.has(account.id)}
              busy={continuity.busyIDs.has(account.id)}
              disabled={!availableIDs.has(account.id)}
              onToggle={(checked) => toggle(account.id, checked)}
            />
          ))}
        </div>
      </div>
      <DataTablePagination
        currentPage={pagination.currentPage}
        totalPages={pagination.totalPages}
        totalItems={rows.length}
        pageSize={pagination.pageSize}
        pageSizes={pageSizes}
        onPageChange={pagination.setCurrentPage}
        onPageSizeChange={pagination.setPageSize}
      />
      <ConfirmActionDialog
        open={confirmation !== null}
        title="确认终端续接检测范围"
        description={`将检测 ${confirmation?.account_ids.length ?? 0} 个账号，每个账号检测 ${confirmation?.rounds ?? 1} 轮并产生 API 用量；结果只用于定位无依据否认终端工具的模型回答，不修改健康分、熔断或调度。`}
        confirmLabel="确认并开始检测"
        pending={continuity.run.isPending}
        onOpenChange={(open) => {
          if (!open) setConfirmation(null);
        }}
        onConfirm={() => {
          if (!confirmation) return;
          continuity.run.mutate(
            {
              targets: confirmation.account_ids.map((accountID) => ({
                account_id: accountID,
                model: confirmation.unified_model.trim(),
              })),
              timeout_seconds: confirmation.timeout_seconds,
              rounds: confirmation.rounds,
            },
            { onSuccess: () => setConfirmation(null) },
          );
        }}
      />
    </form>
  );
}
