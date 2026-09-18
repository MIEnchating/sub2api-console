import { SelectTrafficAccounts } from "@/features/accounts/components/account-traffic";
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
  PrecheckQuestionID,
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
import { defaultAnimationFilters, filterAnimationAccounts } from "../lib/animation-filters";
import type { AnimationForm } from "../lib/animation-schema";
import type { AnimationActivity } from "../lib/animation-task-results";
import { AnimationAccountCard } from "./animation-account-card";
import { ShieldCheck, CheckCheck, ListX, ListChecks, X } from "lucide-react";
import { selectPrecheckAccounts } from "../lib/precheck-selection";
import { allPrecheckQuestions } from "../constants";
import { PrecheckQuestionSelector } from "./precheck-question-selector";

const pageSizes = [6, 12, 24];
const emptyAccounts: AccountStatus[] = [];
const emptyPrecheckResults = new Map<string, AnimationResult>();

export function AnimationSelection(props: {
  accountID?: string;
  form: UseFormReturn<AnimationForm>;
  statuses: Map<string, Task["status"]>;
  activities: Map<string, AnimationActivity>;
  busyIDs: Set<string>;
  results: Map<string, AnimationResult>;
  precheckResults?: Map<string, AnimationResult>;
  precheckStatuses?: Map<string, Task["status"]>;
  onPrecheck?: (value: AnimationForm) => void;
  precheckQuestions?: PrecheckQuestionID[];
  onPrecheckQuestionsChange?: (value: PrecheckQuestionID[]) => void;
  onRetry: (target: AnimationTarget) => void;
  taskRetry: ReactNode;
  accounts: UseQueryResult<AccountStatus[], Error>;
  schedules: UseQueryResult<AnimationSchedule[], Error>;
  pending: boolean;
  onSubmit: (value: AnimationForm) => void;
  onSchedule: (account: AccountStatus) => void;
}): ReactElement {
  const scrollRef = useRef<HTMLDivElement>(null);
  const formScrollRef = useRef<HTMLFormElement>(null);
  const [filters, setFilters] = useState(defaultAnimationFilters);
  const selected = useWatch({ control: props.form.control, name: "account_ids", exact: true });
  const model = useWatch({ control: props.form.control, name: "unified_model", exact: true });
  const accounts = props.accounts.data ?? emptyAccounts;
  const availableSelected = selected.filter(
    (id) => accounts.some((account) => account.id === id) && !props.busyIDs.has(id),
  );
  const filtered = useMemo(
    () =>
      filterAnimationAccounts(
        accounts.filter((account) => !props.accountID || account.id === props.accountID),
        filters,
      ),
    [accounts, filters, props.accountID],
  );
  const precheckQuestions = props.precheckQuestions ?? allPrecheckQuestions;
  const passedIDs = selectPrecheckAccounts(
    filtered,
    props.precheckResults ?? emptyPrecheckResults,
    model,
    "passed",
    props.busyIDs,
    precheckQuestions,
  );
  const notPassedIDs = selectPrecheckAccounts(
    filtered,
    props.precheckResults ?? emptyPrecheckResults,
    model,
    "not_passed",
    props.busyIDs,
    precheckQuestions,
  );
  const pagination = useClientPagination(filtered, 12);
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
    () => new Map(props.schedules.data?.map((schedule) => [schedule.account_id, schedule])),
    [props.schedules.data],
  );
  const form = props.form;
  useEffect(() => {
    if (!props.accounts.data) return;
    const ids = new Set(props.accounts.data.map((account) => account.id));
    const current = form.getValues("account_ids");
    const next = current.filter((id) => ids.has(id));
    if (next.length !== current.length)
      form.setValue("account_ids", next, { shouldValidate: true });
  }, [props.accounts.data, form]);
  const startLabel = props.pending
    ? "正在启动检测"
    : `开始检测（${availableSelected.length} 个账号）`;
  const toggle = useCallback(
    (id: string, checked: boolean): void => {
      const current = form.getValues("account_ids");
      form.setValue(
        "account_ids",
        checked
          ? [...new Set([...current, id])].slice(0, 20)
          : current.filter((value) => value !== id),
        { shouldValidate: true },
      );
    },
    [form],
  );
  return (
    <form
      ref={formScrollRef}
      id="animation-check-form"
      noValidate
      onSubmit={form.handleSubmit(props.onSubmit, (errors) => {
        const message = errors.account_ids?.message;
        if (message) toast.error(message);
      })}
      className="flex h-full min-h-0 flex-col overflow-y-auto md:overflow-hidden"
    >
      <div role="group" aria-label="动画检测设置" className="shrink-0 border-b">
        <div
          role="group"
          aria-label="动画筛选与模型"
          className="flex min-w-0 flex-wrap items-start gap-x-5 gap-y-3 px-3 py-3 sm:px-4"
        >
          <div className="min-w-0 flex-1 basis-[28rem]">
            <AnimationAccountFilters
              accounts={accounts}
              value={filters}
              onChange={(value) => {
                setFilters(value);
                pagination.setCurrentPage(1);
              }}
            />
          </div>
          <AnimationModelSettings form={props.form} pending={false} />
        </div>
        <div
          role="group"
          aria-label="动画检测操作"
          className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2 border-t bg-muted/20 px-3 py-2 sm:px-4"
        >
          <div className="flex shrink-0 items-center gap-2">
            <SelectTrafficAccounts
              accountIDs={filtered
                .filter(
                  (account) =>
                    !props.busyIDs.has(account.id) &&
                    (account.platform == null ||
                      ["openai", "anthropic"].includes(account.platform)),
                )
                .map((account) => account.id)}
              disabled={props.pending || !props.accounts.isSuccess}
              onSelect={(ids) => form.setValue("account_ids", ids, { shouldValidate: true })}
            />
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    aria-label="选择前 20 个账号"
                    disabled={props.accounts.isLoading}
                    onClick={() =>
                      props.form.setValue(
                        "account_ids",
                        filtered
                          .filter(
                            (account) =>
                              !props.busyIDs.has(account.id) &&
                              (account.platform == null ||
                                ["openai", "anthropic"].includes(account.platform)),
                          )
                          .slice(0, 20)
                          .map((account) => account.id),
                        { shouldValidate: true },
                      )
                    }
                  />
                }
              >
                <ListChecks aria-hidden="true" />
              </TooltipTrigger>
              <TooltipContent>选择前 20 个账号</TooltipContent>
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
          {props.onPrecheck ? (
            <div
              role="group"
              aria-label="前置检测操作"
              className="order-2 flex w-full min-w-0 flex-wrap items-center gap-2 sm:order-none sm:w-auto sm:flex-1"
            >
              {props.onPrecheckQuestionsChange ? (
                <PrecheckQuestionSelector
                  value={precheckQuestions}
                  onChange={props.onPrecheckQuestionsChange}
                  disabled={props.pending}
                />
              ) : null}
              <Button
                type="button"
                variant="outline"
                disabled={
                  props.pending ||
                  precheckQuestions.length === 0 ||
                  availableSelected.length === 0 ||
                  !props.accounts.data
                }
                onClick={() =>
                  void form.handleSubmit(
                    (value) => props.onPrecheck?.(value),
                    (errors) => {
                      const message = errors.account_ids?.message;
                      if (message) toast.error(message);
                    },
                  )()
                }
              >
                <ShieldCheck aria-hidden="true" />
                前置检测（{availableSelected.length}）
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={props.pending || passedIDs.length === 0}
                onClick={() => form.setValue("account_ids", passedIDs, { shouldValidate: true })}
              >
                <CheckCheck aria-hidden="true" />
                选择通过（{passedIDs.length}）
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={props.pending || notPassedIDs.length === 0}
                onClick={() => form.setValue("account_ids", notPassedIDs, { shouldValidate: true })}
              >
                <ListX aria-hidden="true" />
                选择不通过（{notPassedIDs.length}）
              </Button>
            </div>
          ) : null}
          <div className="ml-auto flex shrink-0 items-center gap-2">
            {props.taskRetry}
            <div className="w-40 shrink-0">
              <Button
                type="submit"
                className="w-full tabular-nums"
                aria-busy={props.pending}
                disabled={props.pending || availableSelected.length === 0 || !props.accounts.data}
              >
                {startLabel}
              </Button>
            </div>
          </div>
        </div>
      </div>
      <div
        ref={scrollRef}
        role="region"
        aria-label="动画账号卡片"
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
        <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,20rem),1fr))] items-start gap-3">
          {pagination.visibleItems.map((account) => (
            <AnimationAccountCard
              key={account.id}
              account={account}
              result={props.results.get(account.id)}
              precheckResult={props.precheckResults?.get(account.id)}
              precheckStatus={props.precheckStatuses?.get(account.id)}
              taskStatus={props.statuses.get(account.id)}
              activity={props.activities.get(account.id)}
              retryDisabled={props.busyIDs.has(account.id)}
              onRetry={props.onRetry}
              checked={selectedIDs.has(account.id)}
              disabled={
                props.busyIDs.has(account.id) ||
                (!selectedIDs.has(account.id) && selected.length >= 20)
              }
              schedule={schedules.get(account.id)}
              schedulesReady={props.schedules.isSuccess}
              onToggle={toggle}
              onSchedule={props.onSchedule}
            />
          ))}
        </div>
      </div>
      <nav aria-label="动画账号分页" className="shrink-0">
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
