import { ResultModelSelector } from "./result-model-selector";
import { useTodayResults } from "../hooks/use-today-results";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactElement,
  type ReactNode,
} from "react";
import type { UseQueryResult } from "@tanstack/react-query";
import { useWatch, type UseFormReturn } from "react-hook-form";
import type {
  AccountStatus,
  AnimationResult,
  AnimationSchedule,
  AnimationTarget,
  Task,
} from "@/api";
import { AnimationAccountsSkeleton } from "./animation-accounts-skeleton";
import { ContentRetry } from "@/components/content-retry";
import { DataTablePagination } from "@/components/data-table/pagination";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { toast } from "sonner";
import { AnimationModelSettings } from "./animation-model-settings";
import { AnimationAccountFilters } from "./animation-account-filters";
import {
  defaultAnimationFilters,
  filterAnimationAccounts,
  selectableDetectionAccountIDs,
  type AnimationFilters,
} from "../lib/animation-filters";
import type { AnimationForm } from "../lib/animation-schema";
import type { AnimationActivity } from "../lib/animation-task-results";
import { AnimationAccountCard } from "./animation-account-card";
import { ShieldCheck, ListChecks, X } from "lucide-react";
import { sortAccounts } from "@/features/accounts/lib/account-sort";
import { AnimationPromptDialog } from "./animation-prompt-dialog";

const pageSizes = [6, 12, 24];
const emptyAccounts: AccountStatus[] = [];
const emptyResults = new Map<string, AnimationResult>();

export function AnimationSelection(props: {
  mode?: "animation" | "precheck";
  accountID?: string;
  filters?: AnimationFilters;
  onFiltersChange?: (value: AnimationFilters) => void;
  form: UseFormReturn<AnimationForm>;
  statuses: Map<string, Task["status"]>;
  activities: Map<string, AnimationActivity>;
  busyIDs: Set<string>;
  results: Map<string, AnimationResult>;
  modelResults?: Map<string, AnimationResult[]>;
  modelPrecheckResults?: Map<string, AnimationResult[]>;
  precheckResults?: Map<string, AnimationResult>;
  precheckStatuses?: Map<string, Task["status"]>;
  onPrecheck: (value: AnimationForm) => void;
  onRetry: (target: AnimationTarget) => void;
  taskRetry: ReactNode;
  accounts: UseQueryResult<AccountStatus[], Error>;
  schedules: UseQueryResult<AnimationSchedule[], Error>;
  pending: boolean;
  onSubmit: (value: AnimationForm) => void;
  onSchedule: (account: AccountStatus) => void;
  onBatchSchedule?: (accounts: AccountStatus[]) => void;
  onManualPriority: (account: AccountStatus) => void;
}): ReactElement {
  const precheckMode = props.mode === "precheck";
  const animations = useTodayResults(props.results, props.modelResults);
  const prechecks = useTodayResults(
    props.precheckResults ?? emptyResults,
    props.modelPrecheckResults,
  );
  const scrollRef = useRef<HTMLDivElement>(null);
  const formScrollRef = useRef<HTMLFormElement>(null);
  const [localFilters, setLocalFilters] = useState(defaultAnimationFilters);
  const filters = props.filters ?? localFilters;
  const setFilters = props.onFiltersChange ?? setLocalFilters;
  const selected = useWatch({ control: props.form.control, name: "account_ids", exact: true });
  const accounts = props.accounts.data ?? emptyAccounts;
  const [selectedResultModel, setSelectedResultModel] = useState<string>();
  const resultModels = useMemo(() => {
    const results = precheckMode ? prechecks.results : animations.results;
    const modelResults = precheckMode ? prechecks.modelResults : animations.modelResults;
    return [
      ...new Set(
        accounts.flatMap((account) => {
          const choices = modelResults?.get(account.id);
          if (choices?.length) return choices.map((result) => result.model);
          const result = results?.get(account.id);
          return result ? [result.model] : [];
        }),
      ),
    ];
  }, [accounts, precheckMode, animations, prechecks]);
  const resultModel =
    selectedResultModel && resultModels.includes(selectedResultModel)
      ? selectedResultModel
      : resultModels[0];
  const accountIDs = useMemo(() => new Set(accounts.map((account) => account.id)), [accounts]);
  const availableSelected = useMemo(
    () => selected.filter((id) => accountIDs.has(id) && !props.busyIDs.has(id)),
    [selected, accountIDs, props.busyIDs],
  );
  const filtered = useMemo(
    () =>
      sortAccounts(
        filterAnimationAccounts(
          accounts.filter((account) => !props.accountID || account.id === props.accountID),
          filters,
        ),
        filters.sort,
      ),
    [accounts, filters, props.accountID],
  );
  const selectableIDs = useMemo(
    () => selectableDetectionAccountIDs(filtered, props.busyIDs),
    [filtered, props.busyIDs],
  );
  const pagination = useClientPagination(filtered, 12);
  useEffect(() => {
    pagination.setCurrentPage(1);
  }, [filters, props.accountID, pagination.setCurrentPage]);
  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
    const scrollForm = formScrollRef.current;
    const scrollRegion = scrollRef.current;
    if (scrollForm && scrollRegion && scrollForm.scrollTop > 0) {
      scrollForm.scrollTop +=
        scrollRegion.getBoundingClientRect().top - scrollForm.getBoundingClientRect().top;
    }
  }, [pagination.currentPage, pagination.pageSize, filters]);
  const selectedIDs = useMemo(() => new Set(selected), [selected]);
  const schedules = useMemo(
    () =>
      new Map(
        props.schedules.data
          ?.filter((schedule) => (schedule.mode ?? "animation") === (props.mode ?? "animation"))
          .map((schedule) => [schedule.account_id, schedule]),
      ),
    [props.schedules.data, props.mode],
  );
  const form = props.form;
  useEffect(() => {
    if (!props.accounts.data) return;
    const current = form.getValues("account_ids");
    const next = current.filter((id) => accountIDs.has(id));
    if (next.length !== current.length)
      form.setValue("account_ids", next, { shouldValidate: true });
  }, [props.accounts.data, accountIDs, form]);
  let startLabel = props.pending
    ? "正在启动检测"
    : `开始检测（${availableSelected.length} 个账号）`;
  if (precheckMode && !props.pending) startLabel = `前置检测（${availableSelected.length}）`;
  const toggle = useCallback(
    (id: string, checked: boolean): void => {
      const current = form.getValues("account_ids");
      form.setValue(
        "account_ids",
        checked ? [...new Set([...current, id])] : current.filter((value) => value !== id),
        { shouldValidate: true },
      );
    },
    [form],
  );
  return (
    <form
      ref={formScrollRef}
      id={precheckMode ? "precheck-form" : "animation-check-form"}
      noValidate
      onSubmit={form.handleSubmit(precheckMode ? props.onPrecheck : props.onSubmit, (errors) => {
        const message = errors.account_ids?.message;
        if (message) toast.error(message);
      })}
      className="flex h-full min-h-0 flex-col overflow-y-auto md:overflow-hidden"
    >
      <div
        role="group"
        aria-label={precheckMode ? "前置检测设置" : "动画检测设置"}
        className="shrink-0 border-b"
      >
        <div className="min-w-0 px-3 py-3 sm:px-4">
          <AnimationAccountFilters
            resultNavigation={
              resultModels.length > 1 ? (
                <ResultModelSelector
                  compact
                  models={resultModels}
                  value={resultModel}
                  onChange={setSelectedResultModel}
                />
              ) : null
            }
            accounts={accounts}
            value={filters}
            onChange={(value) => {
              setFilters(value);
              pagination.setCurrentPage(1);
            }}
          />
        </div>
        <div
          role="group"
          aria-label={precheckMode ? "前置检测操作" : "动画检测操作"}
          className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 border-t bg-muted/20 px-3 py-2 sm:px-4"
        >
          <div className="flex shrink-0 items-center gap-2">
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    aria-label="全选账号"
                    disabled={
                      props.pending || !props.accounts.isSuccess || selectableIDs.length === 0
                    }
                    onClick={() =>
                      props.form.setValue("account_ids", selectableIDs, { shouldValidate: true })
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
                    aria-label="清空选择"
                    onClick={() => props.form.setValue("account_ids", [], { shouldValidate: true })}
                  />
                }
              >
                <X aria-hidden="true" />
              </TooltipTrigger>
              <TooltipContent>清空选择</TooltipContent>
            </Tooltip>
          </div>
          {!precheckMode ? <AnimationPromptDialog disabled={props.pending} /> : null}
          {props.taskRetry}
          <AnimationModelSettings form={props.form} pending={false} />
          <div className="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-2">
            {props.onBatchSchedule && (
              <Button
                type="button"
                variant="outline"
                disabled={
                  !props.schedules.isSuccess || selected.length === 0 || selected.length > 1000
                }
                onClick={() =>
                  props.onBatchSchedule?.(accounts.filter((account) => selectedIDs.has(account.id)))
                }
              >
                批量自动检测设置
              </Button>
            )}
            <Button
              type="submit"
              className="min-w-40 shrink-0 tabular-nums"
              aria-busy={props.pending}
              disabled={props.pending || availableSelected.length === 0 || !props.accounts.data}
            >
              {precheckMode ? <ShieldCheck aria-hidden="true" /> : null}
              {startLabel}
            </Button>
          </div>
        </div>
      </div>
      <div
        ref={scrollRef}
        role="region"
        aria-label={precheckMode ? "前置检测账号卡片" : "动画账号卡片"}
        className="min-h-80 flex-none shrink-0 overflow-visible bg-muted/10 p-3 sm:p-4 md:min-h-0 md:flex-1 md:overflow-y-auto md:overscroll-contain"
      >
        {props.accounts.isLoading ? <AnimationAccountsSkeleton /> : null}
        {props.accounts.isError && !props.accounts.data ? (
          <ContentRetry
            onRetry={() => void props.accounts.refetch()}
            pending={props.accounts.isFetching}
          />
        ) : null}
        {props.schedules.isError ? (
          <ContentRetry
            onRetry={() => void props.schedules.refetch()}
            pending={props.schedules.isFetching}
          />
        ) : null}
        {props.accounts.isSuccess && filtered.length === 0 ? (
          <p className="text-muted-foreground text-sm">没有匹配的账号</p>
        ) : null}
        <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,20rem),1fr))] items-stretch gap-3">
          {pagination.visibleItems.map((account) => (
            <AnimationAccountCard
              key={account.id}
              mode={precheckMode ? "precheck" : "animation"}
              account={account}
              selectedModel={resultModel}
              result={animations.results.get(account.id)}
              modelResults={animations.modelResults.get(account.id)}
              modelPrecheckResults={prechecks.modelResults.get(account.id)}
              precheckResult={prechecks.results.get(account.id)}
              precheckStatus={
                prechecks.results.has(account.id) ||
                !props.precheckResults?.has(account.id) ||
                props.busyIDs.has(account.id)
                  ? props.precheckStatuses?.get(account.id)
                  : undefined
              }
              taskStatus={
                animations.results.has(account.id) ||
                !props.results.has(account.id) ||
                props.busyIDs.has(account.id)
                  ? props.statuses.get(account.id)
                  : undefined
              }
              activity={props.activities.get(account.id)}
              retryDisabled={props.busyIDs.has(account.id)}
              onRetry={props.onRetry}
              checked={selectedIDs.has(account.id)}
              disabled={props.busyIDs.has(account.id)}
              schedule={schedules.get(account.id)}
              schedulesReady={props.schedules.isSuccess}
              onToggle={toggle}
              onSchedule={props.onSchedule}
              onManualPriority={props.onManualPriority}
            />
          ))}
        </div>
      </div>
      <nav aria-label={precheckMode ? "前置检测账号分页" : "动画账号分页"} className="shrink-0">
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={filtered.length}
          pageSize={pagination.pageSize}
          pageSizes={pageSizes}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      </nav>
    </form>
  );
}
