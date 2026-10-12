import { useEffect, useMemo, useRef, useState, type ReactElement } from "react";
import { CircleCheck, CircleX } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { Task } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { useClientPagination } from "@/hooks/use-client-pagination";

const automationResultPageSize = 20;

type AutomationItem = {
  account_id?: string;
  channel_id?: string;
  channel_name?: string;
  models?: unknown;
  planned?: unknown;
  added?: unknown;
  existing?: unknown;
  status: string;
  message?: string;
};

export function automationModels(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function resultItems(task: Task): AutomationItem[] {
  if (!Array.isArray(task.result.items)) return [];
  return task.result.items.filter(
    (item): item is AutomationItem =>
      typeof item === "object" &&
      item !== null &&
      typeof item.status === "string" &&
      (typeof item.account_id === "string" || typeof item.channel_id === "string"),
  );
}

type PricePlan = {
  model: string;
  input_ratio: string;
  completion_ratio: string;
  billing_expr?: string;
  cache_ratio?: string;
  create_cache_ratio?: string;
  image_ratio?: string;
};

function pricePlan(task: Task): PricePlan[] {
  if (!Array.isArray(task.result.pricing_plan)) return [];
  return task.result.pricing_plan.filter(
    (item): item is PricePlan =>
      typeof item === "object" &&
      item !== null &&
      typeof item.model === "string" &&
      typeof item.input_ratio === "string" &&
      typeof item.completion_ratio === "string",
  );
}

function accountOutcome(item: AutomationItem, preview: boolean): string {
  if (item.status !== "succeeded") return preview ? "目录读取失败" : "目录同步失败";
  return preview ? "目录已读取" : "目录已刷新";
}

function ModelList(props: { label: string; models: string[] }): ReactElement | null {
  if (props.models.length === 0) return null;
  return (
    <details className="min-w-0 text-xs">
      <summary className="cursor-pointer py-1">
        {props.label}（{props.models.length} 个）
      </summary>
      <p className="max-h-36 overflow-y-auto break-all text-muted-foreground">
        {props.models.join("、")}
      </p>
    </details>
  );
}

type AutomationResultsProps = {
  task: Task;
  showSummary?: boolean;
};

export function AccountModelAutomationResults(props: AutomationResultsProps): ReactElement {
  return <AutomationResultsContent key={props.task.id} {...props} />;
}

function AutomationResultsContent(props: AutomationResultsProps): ReactElement {
  const task = props.task;
  const preview = task.result.dry_run === true;
  const scheduledPreview = preview && typeof task.result.schedule_id === "string";
  const running = ["queued", "running", "waiting_input"].includes(task.status);
  const reading = running && task.result.phase === "discovering";
  let title = "执行结果";
  if (preview) title = scheduledPreview ? "执行前检查" : "预览结果（尚未执行）";
  const currentFormat = task.result.result_version === 2;
  const items = useMemo(() => resultItems(task), [task]);
  const accounts = useMemo(() => items.filter((item) => item.account_id), [items]);
  const channels = useMemo(() => items.filter((item) => item.channel_id), [items]);
  const pricingPlan = useMemo(() => pricePlan(task), [task]);
  const [accountFilter, setAccountFilter] = useState<"all" | "failed" | "succeeded" | null>(null);
  const failedAccounts = useMemo(
    () => accounts.filter((item) => item.status === "failed"),
    [accounts],
  );
  const successfulAccounts = useMemo(
    () => accounts.filter((item) => item.status === "succeeded"),
    [accounts],
  );
  const activeFilter = accountFilter ?? (failedAccounts.length > 0 ? "failed" : "all");
  let filteredAccounts = accounts;
  if (activeFilter === "failed") filteredAccounts = failedAccounts;
  if (activeFilter === "succeeded") filteredAccounts = successfulAccounts;
  const accountPagination = useResultPagination(filteredAccounts);
  const channelPagination = useResultPagination(channels);
  const pricingPagination = useResultPagination(pricingPlan);
  const models = useMemo(() => automationModels(task.result.models), [task.result.models]);
  const selectedModels = useMemo(
    () => automationModels(task.result.selected_models),
    [task.result.selected_models],
  );
  return (
    <section aria-label={preview ? "运营预览结果" : "运营执行结果"} className="grid min-w-0 gap-4">
      <div>
        <h3 className="text-sm font-semibold">{title}</h3>
        {preview && !scheduledPreview && (
          <p className="mt-1 text-xs text-muted-foreground">
            本次仅检查影响范围，未同步账号目录、上架模型或修改价格。
          </p>
        )}
      </div>
      {props.showSummary !== false && (currentFormat || task.status !== "succeeded") && (
        <p role="status" className="text-sm break-all">
          {task.message}
        </p>
      )}
      <p className="text-sm">
        {reading ? "正在汇总账号可用模型" : `读取到模型 ${models.length} 个`}
        {currentFormat && <> · 选定上架 {selectedModels.length} 个</>}
      </p>
      {accounts.length > 0 && (
        <section aria-label="账号模型目录" className="grid min-w-0 gap-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h4 className="text-sm font-medium">账号模型目录</h4>
            <div role="group" aria-label="账号结果筛选" className="flex flex-wrap gap-1">
              {(
                [
                  { value: "all", label: "全部", count: accounts.length },
                  { value: "failed", label: "失败", count: failedAccounts.length },
                  { value: "succeeded", label: "成功", count: successfulAccounts.length },
                ] as const
              ).map((filter) => (
                <Button
                  key={filter.value}
                  type="button"
                  variant={activeFilter === filter.value ? "secondary" : "ghost"}
                  aria-pressed={activeFilter === filter.value}
                  onClick={() => {
                    setAccountFilter(filter.value);
                    accountPagination.setCurrentPage(1);
                    if (accountPagination.scrollRef.current)
                      accountPagination.scrollRef.current.scrollTop = 0;
                  }}
                >
                  {filter.label} {filter.count}
                </Button>
              ))}
            </div>
          </div>
          <div
            ref={accountPagination.scrollRef}
            role="list"
            className="max-h-96 overflow-y-auto rounded-lg border bg-background"
          >
            {filteredAccounts.length === 0 && (
              <p className="p-6 text-center text-sm text-muted-foreground">
                {activeFilter === "failed" ? "没有失败的账号" : "没有成功的账号"}
              </p>
            )}
            {accountPagination.visibleItems.map((item, index) => (
              <div
                role="listitem"
                key={item.account_id + "-" + index}
                className="min-w-0 border-b p-3 text-sm last:border-b-0"
              >
                <div className="flex min-w-0 items-start gap-2">
                  {item.status === "failed" ? (
                    <CircleX
                      className="mt-0.5 size-4 shrink-0 text-destructive"
                      aria-hidden="true"
                    />
                  ) : (
                    <CircleCheck
                      className="mt-0.5 size-4 shrink-0 text-success"
                      aria-hidden="true"
                    />
                  )}
                  <p className="min-w-0 wrap-anywhere font-medium">
                    账号 {item.account_id} · {accountOutcome(item, preview)}
                  </p>
                </div>
                <ModelList label="查看账号可用模型" models={automationModels(item.models)} />
                {item.message && (
                  <p
                    className={cn(
                      "mt-1 text-xs leading-5 wrap-anywhere",
                      item.status === "failed" ? "text-destructive" : "text-muted-foreground",
                    )}
                  >
                    {item.message}
                  </p>
                )}
              </div>
            ))}
          </div>
          {filteredAccounts.length >
            Math.min(automationResultPageSize, accountPagination.pageSize) && (
            <DataTablePagination
              compact
              currentPage={accountPagination.currentPage}
              totalPages={accountPagination.totalPages}
              totalItems={filteredAccounts.length}
              pageSize={accountPagination.pageSize}
              onPageChange={accountPagination.setCurrentPage}
              onPageSizeChange={accountPagination.setPageSize}
            />
          )}
        </section>
      )}
      {channels.length > 0 && (
        <section aria-label="渠道模型上架" className="grid min-w-0 gap-2">
          <h4 className="text-sm font-medium">渠道模型上架</h4>
          <div
            ref={channelPagination.scrollRef}
            role="list"
            className="max-h-52 overflow-y-auto rounded border"
          >
            {channelPagination.visibleItems.map((item, index) => (
              <div
                role="listitem"
                key={item.channel_id + "-" + index}
                className="min-w-0 border-b p-3 text-sm last:border-b-0"
              >
                <p className="break-all">
                  渠道 {item.channel_name || item.channel_id}
                  {item.channel_name && <>（{item.channel_id}）</>}
                </p>
                <ModelList
                  label="待上架"
                  models={preview ? automationModels(item.planned ?? item.added) : []}
                />
                <ModelList
                  label="已新增"
                  models={!preview && currentFormat ? automationModels(item.added) : []}
                />
                <ModelList label="已存在，跳过" models={automationModels(item.existing)} />
                {item.status === "failed" && (
                  <p className="text-destructive text-xs">
                    上架失败：{item.message || "请核对渠道模型后重试"}
                  </p>
                )}
                {item.status !== "failed" && item.message && (
                  <p className="text-xs text-muted-foreground break-all">{item.message}</p>
                )}
                {!preview && !currentFormat && (
                  <p className="text-xs text-muted-foreground">
                    历史记录未区分计划与实际新增，请核对渠道当前模型。
                  </p>
                )}
              </div>
            ))}
          </div>
          {channels.length > Math.min(automationResultPageSize, channelPagination.pageSize) && (
            <DataTablePagination
              compact
              currentPage={channelPagination.currentPage}
              totalPages={channelPagination.totalPages}
              totalItems={channels.length}
              pageSize={channelPagination.pageSize}
              onPageChange={channelPagination.setCurrentPage}
              onPageSizeChange={channelPagination.setPageSize}
            />
          )}
        </section>
      )}
      {channels.length === 0 && running && (
        <p className="text-xs text-muted-foreground">
          正在检查执行范围，渠道上架结果将在检查完成后显示。
        </p>
      )}
      {((channels.length === 0 && !running) || task.result.pricing_enabled === false) && (
        <ul
          aria-label="执行范围说明"
          className="flex min-w-0 flex-wrap gap-x-4 gap-y-1 rounded-md bg-muted/30 px-3 py-2 text-xs leading-5 text-muted-foreground"
        >
          {channels.length === 0 && !running && <li>本次没有渠道上架记录。</li>}
          {task.result.pricing_enabled === false && <li>本次不修改模型价格。</li>}
        </ul>
      )}
      {typeof task.result.pricing_applied_models === "number" &&
        !preview &&
        task.result.pricing_enabled === true && (
          <p className="text-sm">价格已同步 {task.result.pricing_applied_models} 个模型</p>
        )}
      {typeof task.result.pricing_error === "string" && (
        <p className="text-sm text-destructive">价格同步失败：{task.result.pricing_error}</p>
      )}
      {typeof task.result.pricing_missing === "number" && task.result.pricing_missing > 0 && (
        <p className="text-xs text-muted-foreground">
          {task.result.pricing_missing} 个模型缺少可用参考价，不会修改其价格。
        </p>
      )}
      {pricingPlan.length > 0 && (
        <section aria-label="模型定价方案" className="grid min-w-0 gap-2">
          <div
            ref={pricingPagination.scrollRef}
            className="max-h-40 overflow-y-auto rounded border"
          >
            {pricingPagination.visibleItems.map((price) => (
              <div key={price.model} className="border-b p-2 text-xs last:border-b-0">
                <p className="break-all font-medium">{price.model}</p>
                <p>
                  输入倍率 {price.input_ratio} · 输出倍率 {price.completion_ratio}
                </p>
                {price.cache_ratio && <p>缓存倍率 {price.cache_ratio}</p>}
                {price.create_cache_ratio && <p>创建缓存倍率 {price.create_cache_ratio}</p>}
                {price.image_ratio && <p>图片倍率 {price.image_ratio}</p>}
                {price.billing_expr && (
                  <p className="break-all">分时计费表达式：{price.billing_expr}</p>
                )}
              </div>
            ))}
          </div>
          {pricingPlan.length > Math.min(automationResultPageSize, pricingPagination.pageSize) && (
            <DataTablePagination
              currentPage={pricingPagination.currentPage}
              totalPages={pricingPagination.totalPages}
              totalItems={pricingPlan.length}
              pageSize={pricingPagination.pageSize}
              onPageChange={pricingPagination.setCurrentPage}
              onPageSizeChange={pricingPagination.setPageSize}
            />
          )}
        </section>
      )}
    </section>
  );
}

type TaskGroupItem = {
  schedule_id: string;
  schedule_name: string;
  preview_task_id?: string;
  execution_task_id?: string;
  status: string;
  message?: string;
};

function taskGroupItems(task: Task): TaskGroupItem[] {
  if (!Array.isArray(task.result.items)) return [];
  return task.result.items.filter(
    (item): item is TaskGroupItem =>
      typeof item === "object" &&
      item !== null &&
      typeof item.schedule_id === "string" &&
      typeof item.schedule_name === "string" &&
      typeof item.status === "string",
  );
}

function taskGroupStatus(status: string): string {
  const labels: Record<string, string> = {
    queued: "排队中",
    running: "进行中",
    succeeded: "已成功",
    partial: "部分完成",
    failed: "已失败",
    cancelled: "已取消",
  };
  return labels[status] ?? status;
}

export function AccountModelAutomationGroupResults(props: { task: Task }): ReactElement {
  return <AutomationGroupResultsContent key={props.task.id} task={props.task} />;
}

function AutomationGroupResultsContent(props: { task: Task }): ReactElement {
  const items = useMemo(() => taskGroupItems(props.task), [props.task]);
  const pagination = useResultPagination(items);
  return (
    <section
      aria-label="运营任务组结果"
      className="grid min-w-0 gap-3 rounded-lg border bg-muted/20 p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">任务组成员结果</h3>
        <span className="text-muted-foreground text-xs">已完成 {items.length} 个任务</span>
      </div>
      {items.length === 0 && (
        <p className="text-muted-foreground text-sm">正在等待第一个运营任务开始。</p>
      )}
      {items.length > 0 && (
        <div
          ref={pagination.scrollRef}
          role="list"
          className="max-h-80 overflow-y-auto rounded border bg-background"
        >
          {pagination.visibleItems.map((item) => (
            <div
              key={item.schedule_id}
              role="listitem"
              className="grid min-w-0 gap-1 border-b p-3 text-sm last:border-b-0"
            >
              <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
                <span className="min-w-0 truncate font-medium">{item.schedule_name}</span>
                <span className="text-muted-foreground text-xs">
                  {taskGroupStatus(item.status)}
                </span>
              </div>
              {item.message && (
                <p className="text-muted-foreground wrap-anywhere text-xs">{item.message}</p>
              )}
              {(item.preview_task_id || item.execution_task_id) && (
                <p className="text-muted-foreground wrap-anywhere font-mono text-xs">
                  {item.preview_task_id ? `预览 ${item.preview_task_id}` : ""}
                  {item.preview_task_id && item.execution_task_id ? " · " : ""}
                  {item.execution_task_id ? `执行 ${item.execution_task_id}` : ""}
                </p>
              )}
            </div>
          ))}
        </div>
      )}
      {items.length > Math.min(automationResultPageSize, pagination.pageSize) && (
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={items.length}
          pageSize={pagination.pageSize}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      )}
    </section>
  );
}

function useResultPagination<T>(items: readonly T[]) {
  const pagination = useClientPagination(items, automationResultPageSize);
  const scrollRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
  }, [pagination.currentPage, pagination.pageSize]);
  return { ...pagination, scrollRef };
}
