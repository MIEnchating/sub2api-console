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
import { toast } from "sonner";
import { AnimationModelSettings } from "./animation-model-settings";
import { AnimationAccountFilters } from "./animation-account-filters";
import { defaultAnimationFilters, filterAnimationAccounts } from "../lib/animation-filters";
import type { AnimationForm } from "../lib/animation-schema";
import type { AnimationActivity } from "../lib/animation-task-results";
import { AnimationAccountCard } from "./animation-account-card";

const pageSizes = [6, 12, 24];
const emptyAccounts: AccountStatus[] = [];

export function AnimationSelection(props: {
  form: UseFormReturn<AnimationForm>;
  statuses: Map<string, Task["status"]>;
  activities: Map<string, AnimationActivity>;
  busyIDs: Set<string>;
  results: Map<string, AnimationResult>;
  onRetry: (target: AnimationTarget) => void;
  taskRetry: ReactNode;
  accounts: UseQueryResult<AccountStatus[], Error>;
  schedules: UseQueryResult<AnimationSchedule[], Error>;
  pending: boolean;
  onSubmit: (value: AnimationForm) => void;
  onSchedule: (account: AccountStatus) => void;
}): ReactElement {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [filters, setFilters] = useState(defaultAnimationFilters);
  const selected = useWatch({ control: props.form.control, name: "account_ids", exact: true });
  const accounts = props.accounts.data ?? emptyAccounts;
  const availableSelected = selected.filter((id) => !props.busyIDs.has(id));
  const filtered = useMemo(() => filterAnimationAccounts(accounts, filters), [accounts, filters]);
  const pagination = useClientPagination(filtered, 12);
  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
  }, [pagination.currentPage, pagination.pageSize, filters]);
  const selectedIDs = useMemo(() => new Set(selected), [selected]);
  const schedules = useMemo(
    () => new Map(props.schedules.data?.map((schedule) => [schedule.account_id, schedule])),
    [props.schedules.data],
  );
  const form = props.form;
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
      id="animation-check-form"
      onSubmit={form.handleSubmit(props.onSubmit, (errors) => {
        const message =
          errors.account_ids?.message ??
          errors.unified_model?.message ??
          errors.timeout_seconds?.message;
        if (message) toast.error(message);
      })}
      className="flex h-full min-h-0 flex-col overflow-hidden"
    >
      <div role="group" aria-label="动画检测设置" className="shrink-0 border-b">
        <div className="px-3 py-2.5 sm:px-4">
          <AnimationAccountFilters
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
          aria-label="动画检测操作"
          className="flex min-w-0 items-start gap-3 border-t bg-muted/20 px-3 py-2.5 sm:px-4"
        >
          <div className="min-w-0 flex-1">
            <AnimationModelSettings form={props.form} pending={false}>
              <Button
                type="button"
                variant="outline"
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
              >
                选择前 20 个账号
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => props.form.setValue("account_ids", [], { shouldValidate: true })}
              >
                清空选择
              </Button>
            </AnimationModelSettings>
          </div>
          <div className="flex shrink-0 items-center gap-2">
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
        className="min-h-0 flex-1 overflow-y-auto overscroll-contain bg-muted/10 p-3 sm:p-4"
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
        <div className="grid items-start gap-3 md:grid-cols-2 lg:grid-cols-4">
          {pagination.visibleItems.map((account) => (
            <AnimationAccountCard
              key={account.id}
              account={account}
              result={props.results.get(account.id)}
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
