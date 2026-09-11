import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { useForm } from "react-hook-form";
import {
  CircleDollarSign,
  FileText,
  GitCompareArrows,
  RefreshCw,
  RotateCcw,
  Search,
  TrendingDown,
  TrendingUp,
  Upload,
} from "lucide-react";

import type {
  ModelPriceCatalog,
  NewAPIModelPrice,
  NewAPIRemoteSnapshot,
  Sub2APIModelPrice,
} from "@/api";
import {
  BatchModelPriceDialog,
  PriceSelectionCheckbox,
  type BatchModelPricePreview,
} from "./batch-model-price-dialog";
import { DataTablePagination } from "@/components/data-table/pagination";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { StatusBadge } from "@/components/status-badge";
import { modelPriceSourceLabels } from "../constants";
import { ModelPriceSelectionToolbar } from "./model-price-selection-toolbar";
import { Button } from "@/components/ui/button";
import { TableOverflowTooltip } from "@/components/ui/table-overflow-tooltip";
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
import { useClientPagination } from "@/hooks/use-client-pagination";
import { notifyOperationError } from "@/lib/operation-feedback";
import {
  adjustNewAPIModelPrice,
  type ModelPriceAdjustmentDirection,
} from "../lib/model-price-adjustment";
import {
  modelPriceAdjustmentSchema,
  type ModelPriceAdjustmentValues,
} from "../lib/model-price-adjustment-schema";
import {
  formatModelPriceNumber,
  modelPriceNumbersEqual,
  pricePerMillion,
} from "../lib/pricing-number";
import { timePricingDescription, timePricingExpression } from "../lib/time-pricing";
import { TimePriceValues } from "./time-price-values";
import { OfficialTierValues } from "./official-tier-values";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

type PriceProps = {
  models: NewAPIModelPrice[];
  unsetModels?: NewAPIModelPrice[];
  managementPrices?: Sub2APIModelPrice[];
  managementPricesPending?: boolean;
  managementPricesError?: string;
  managementPricesStale?: boolean;
  managementPricesWarning?: string;
  managementPricesFetchedAt?: string;
  onRefreshManagementPrices?: () => void;
  onViewManagementPrices?: () => void;
  onCompareManagementPrices?: () => void;
  onViewRawPricingSource?: () => void;
  onWriteManagementPrice?: (price: Sub2APIModelPrice) => void;
  onWriteModelPrice?: (price: NewAPIModelPrice, action: string) => Promise<boolean>;
  onSyncModelPrice?: (model: string) => Promise<boolean>;
  onLoadManagementPrices?: () => Promise<ModelPriceCatalog>;
  onWriteModelPrices?: (prices: NewAPIModelPrice[]) => Promise<NewAPIRemoteSnapshot>;
  writingManagementPrice?: string;
  writtenManagementPrice?: NewAPIModelPrice | null;
  onWrittenManagementPriceOpenChange?: (open: boolean) => void;
};

type PriceTab = "models" | "unset" | "remote";

type PriceAdjustmentRequest = {
  price: NewAPIModelPrice;
  direction: ModelPriceAdjustmentDirection;
};

type PendingModelAction = {
  model: string;
  kind: "write" | "sync";
};

export type PriceDifferenceSelection = {
  configured: NewAPIModelPrice;
  remote: Sub2APIModelPrice;
};

export function NewAPIModelPrices(props: PriceProps) {
  const [tab, setTab] = useState<PriceTab>("models");
  const [search, setSearch] = useState("");
  const [remoteSearch, setRemoteSearch] = useState("");
  const [comparisonRequested, setComparisonRequested] = useState(false);
  const [differenceSelection, setDifferenceSelection] = useState<PriceDifferenceSelection | null>(
    null,
  );
  const [adjustmentRequest, setAdjustmentRequest] = useState<PriceAdjustmentRequest | null>(null);
  const [restorePrices, setRestorePrices] = useState<Record<string, NewAPIModelPrice>>({});
  const [pendingModelAction, setPendingModelAction] = useState<PendingModelAction | null>(null);
  const [selectedModels, setSelectedModels] = useState<Set<string>>(new Set());
  const [batchPreview, setBatchPreview] = useState<BatchModelPricePreview[] | null>(null);
  const [batchPreparing, setBatchPreparing] = useState(false);
  const [batchWriting, setBatchWriting] = useState(false);
  const [batchFailed, setBatchFailed] = useState(false);
  const batchPreviewRequest = useRef(0);
  const [batchResults, setBatchResults] = useState<Record<string, string> | null>(null);
  const batchEnabled = Boolean(props.onLoadManagementPrices && props.onWriteModelPrices);
  const selectionBusy =
    batchPreparing ||
    batchWriting ||
    pendingModelAction !== null ||
    Boolean(props.managementPricesPending) ||
    Boolean(props.writingManagementPrice);
  const rows = useMemo(() => {
    return [...props.models]
      .sort((left, right) => left.model.localeCompare(right.model))
      .map((model) => ({ model: model.model, configured: model }));
  }, [props.models]);
  const unsetRows = useMemo(
    () =>
      [...(props.unsetModels ?? [])]
        .sort((left, right) => left.model.localeCompare(right.model))
        .map((model) => ({ model: model.model, configured: model })),
    [props.unsetModels],
  );

  const activeRows = tab === "unset" ? unsetRows : rows;
  const filteredRows = useMemo(() => {
    const query = search.trim().toLocaleLowerCase();
    return query
      ? activeRows.filter((row) => row.model.toLocaleLowerCase().includes(query))
      : activeRows;
  }, [activeRows, search]);
  const filteredRemotePrices = useMemo(
    () => filterRemoteModelPrices(props.managementPrices ?? [], remoteSearch),
    [props.managementPrices, remoteSearch],
  );
  const pagination = useClientPagination(filteredRows);

  function changeTab(next: PriceTab): void {
    if (next !== tab) setSelectedModels(new Set());
    setTab(next);
  }

  function selectModels(models: string[], checked: boolean): void {
    setSelectedModels((current) => {
      const next = new Set(current);
      for (const model of models) {
        if (checked) next.add(model);
        else next.delete(model);
      }
      return next;
    });
  }

  async function prepareBatchSync(): Promise<void> {
    if (!props.onLoadManagementPrices || selectedModels.size === 0 || selectedModels.size > 1000)
      return;
    const requestID = ++batchPreviewRequest.current;
    setBatchPreview([]);
    setBatchResults(null);
    setBatchFailed(false);
    setBatchPreparing(true);
    try {
      const catalog = await props.onLoadManagementPrices();
      if (requestID !== batchPreviewRequest.current) return;
      if (catalog.stale)
        throw new Error("参考价格已过期或刷新不完整，请先强制刷新参考价格后再批量同步");
      const configured = new Map(
        [...props.models, ...(props.unsetModels ?? [])].map((price) => [price.model, price]),
      );
      setBatchPreview(
        [...selectedModels].sort().map((model) => {
          const reference = matchingRemoteModelPrice(catalog.models, model);
          if (!reference) return { model, reason: "跳过：参考价未找到", differences: [] };
          if (!remotePriceSupportsNewAPIWrite(reference))
            return { model, reason: "跳过：不支持此计费格式", differences: [] };
          const current = configured.get(model) ?? { model, input_ratio: "", completion_ratio: "" };
          return {
            model,
            reference,
            price: remotePriceToNewAPIModelPrice(reference),
            differences: modelPriceDifferenceRows(current, reference),
          };
        }),
      );
    } catch (error) {
      if (requestID !== batchPreviewRequest.current) return;
      setBatchFailed(true);
      notifyOperationError(error, "参考价格读取失败，请重试");
    } finally {
      if (requestID === batchPreviewRequest.current) setBatchPreparing(false);
    }
  }

  function closeBatchPreview(): void {
    batchPreviewRequest.current += 1;
    setBatchPreview(null);
    setBatchPreparing(false);
    setBatchFailed(false);
  }

  async function confirmBatchSync(): Promise<void> {
    if (!props.onWriteModelPrices || !batchPreview || batchWriting) return;
    const prices = batchPreview.flatMap((row) => (row.price ? [row.price] : []));
    if (prices.length === 0) return;
    setBatchWriting(true);
    setBatchFailed(false);
    try {
      const result = await props.onWriteModelPrices(prices);
      const actual = new Map(result.models.map((price) => [price.model, price]));
      const successful: string[] = [];
      const results: Record<string, string> = {};
      for (const row of batchPreview) {
        if (!row.reference || !row.price) {
          results[row.model] = row.reason ?? "已跳过";
          continue;
        }
        const readback = actual.get(row.model);
        if (readback && newAPIPriceComparisonStatus(readback, [row.reference]) === "matched") {
          results[row.model] = "同步成功并已读回";
          successful.push(row.model);
          const before = props.models.find((price) => price.model === row.model);
          if (before) rememberRestorePrice(before);
        } else {
          results[row.model] = "已提交，读回价格未匹配，请核对";
        }
      }
      setBatchResults(results);
      selectModels(successful, false);
    } catch (error) {
      setBatchFailed(true);
      notifyOperationError(error, "批量同步失败，请核对平台后重试");
    } finally {
      setBatchWriting(false);
    }
  }

  function showRemotePrices(model: string) {
    setRemoteSearch(model);
    changeTab("remote");
    props.onViewManagementPrices?.();
  }

  function rememberRestorePrice(price: NewAPIModelPrice) {
    setRestorePrices((current) => {
      if (current[price.model]) return current;
      return { ...current, [price.model]: { ...price } };
    });
  }

  async function writeModelPrice(price: NewAPIModelPrice, action: string): Promise<boolean> {
    if (!props.onWriteModelPrice) return false;
    setPendingModelAction({ model: price.model, kind: "write" });
    try {
      return await props.onWriteModelPrice(price, action);
    } catch {
      return false;
    } finally {
      setPendingModelAction(null);
    }
  }

  async function submitAdjustment(
    request: PriceAdjustmentRequest,
    values: ModelPriceAdjustmentValues,
  ): Promise<void> {
    const adjusted = adjustNewAPIModelPrice(request.price, request.direction, values.percentage);
    const action = `${request.direction === "increase" ? "上调" : "下调"} ${values.percentage}%`;
    if (!(await writeModelPrice(adjusted, action))) return;
    rememberRestorePrice(request.price);
    setAdjustmentRequest(null);
  }

  async function restoreModelPrice(model: string): Promise<void> {
    const restorePrice = restorePrices[model];
    if (!restorePrice || !(await writeModelPrice(restorePrice, "还原"))) return;
    setRestorePrices((current) => {
      const next = { ...current };
      delete next[model];
      return next;
    });
  }

  async function syncModelPrice(price: NewAPIModelPrice): Promise<void> {
    if (!props.onSyncModelPrice) return;
    setPendingModelAction({ model: price.model, kind: "sync" });
    try {
      if (await props.onSyncModelPrice(price.model)) rememberRestorePrice(price);
    } catch {
      // The page mutation owns the user-facing error notification.
    } finally {
      setPendingModelAction(null);
    }
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <TableFilterToolbar aria-label="模型价格筛选与操作">
        <label className="relative w-full sm:w-56">
          <Search
            className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
            aria-hidden="true"
          />
          <Input
            className="pl-8"
            value={tab === "remote" ? remoteSearch : search}
            onChange={(event) => {
              if (tab === "remote") {
                setRemoteSearch(event.target.value);
                return;
              }
              setSearch(event.target.value);
            }}
            placeholder="搜索模型"
            aria-label={tab === "remote" ? "搜索远程模型" : "搜索模型"}
          />
        </label>
        <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
          {props.onRefreshManagementPrices ? (
            <TableActionButton
              label="强制刷新参考价格"
              ariaLabel="强制刷新参考价格"
              disabled={props.managementPricesPending}
              onClick={props.onRefreshManagementPrices}
            >
              <RefreshCw
                className={props.managementPricesPending ? "animate-spin" : undefined}
                aria-hidden="true"
              />
            </TableActionButton>
          ) : null}
          {props.onViewRawPricingSource ? (
            <Button variant="outline" onClick={props.onViewRawPricingSource}>
              <FileText aria-hidden="true" />
              查看原始价卡
            </Button>
          ) : null}
          {props.onCompareManagementPrices ? (
            <Button
              variant="outline"
              disabled={props.managementPricesPending}
              onClick={() => {
                setComparisonRequested(true);
                props.onCompareManagementPrices?.();
              }}
            >
              <GitCompareArrows aria-hidden="true" />
              {comparisonRequested && props.managementPricesPending && !props.managementPrices
                ? "正在比较"
                : "比较模型价格"}
            </Button>
          ) : null}
        </div>
      </TableFilterToolbar>
      {props.managementPricesFetchedAt ||
      props.managementPricesStale ||
      props.managementPricesWarning ||
      props.managementPricesError ? (
        <div
          className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground"
          role="status"
        >
          {props.managementPricesFetchedAt ? (
            <span>
              价格缓存更新：{new Date(props.managementPricesFetchedAt).toLocaleString("zh-CN")}
            </span>
          ) : null}
          {props.managementPricesStale ? (
            <StatusBadge label="缓存过期或刷新不完整" variant="warning" />
          ) : null}
          {props.managementPricesWarning ? <span>{props.managementPricesWarning}</span> : null}
        </div>
      ) : null}
      <SegmentedControl role="tablist" aria-label="价格分类">
        <SegmentedControlItem
          id="price-tab-models"
          role="tab"
          aria-controls="price-panel-models"
          selected={tab === "models"}
          onClick={() => changeTab("models")}
        >
          模型价格
        </SegmentedControlItem>
        <SegmentedControlItem
          id="price-tab-unset"
          role="tab"
          aria-controls="price-panel-unset"
          selected={tab === "unset"}
          onClick={() => changeTab("unset")}
        >
          未设置模型价格
        </SegmentedControlItem>
        <SegmentedControlItem
          id="price-tab-remote"
          role="tab"
          aria-controls="price-panel-remote"
          selected={tab === "remote"}
          onClick={() => showRemotePrices("")}
        >
          远程模型价格
        </SegmentedControlItem>
      </SegmentedControl>
      <div
        id={`price-panel-${tab}`}
        role="tabpanel"
        aria-labelledby={`price-tab-${tab}`}
        className="contents"
      >
        {tab === "remote" ? (
          <RemoteModelPricesTable
            key={remoteSearch}
            prices={filteredRemotePrices}
            pending={props.managementPricesPending ?? false}
            error={props.managementPricesError ?? ""}
            filtered={remoteSearch !== ""}
            writingModel={props.writingManagementPrice}
            onWritePrice={props.onWriteManagementPrice}
            selectedModels={batchEnabled ? selectedModels : undefined}
            onSelectModels={selectModels}
            selectionDisabled={selectionBusy}
          />
        ) : null}
        {(tab === "models" || tab === "unset") && activeRows.length === 0 ? (
          <div className="text-muted-foreground flex min-h-52 flex-col items-center justify-center gap-2 px-6 text-sm">
            <CircleDollarSign className="size-8 opacity-45" aria-hidden="true" />
            <span>{tab === "unset" ? "没有未设置价格的模型" : "尚未读取到模型价格"}</span>
          </div>
        ) : null}
        {(tab === "models" || tab === "unset") && activeRows.length > 0 ? (
          <DataTablePanel className="flex-1">
            <Table containerClassName="min-h-0 flex-1 overflow-auto">
              <TableHeader className="sticky top-0 z-10 bg-background">
                <TableRow>
                  {batchEnabled ? (
                    <TableHead className="w-10">
                      <PriceSelectionCheckbox
                        models={pagination.visibleItems.map((row) => row.model)}
                        selected={selectedModels}
                        label="选择本页模型"
                        disabled={selectionBusy}
                        onChange={selectModels}
                      />
                    </TableHead>
                  ) : null}
                  <TableHead className="min-w-52">模型</TableHead>
                  <TableHead className="w-40 text-right">输入价格</TableHead>
                  <TableHead className="w-40 text-right">输出价格</TableHead>
                  <TableHead className="w-40 text-right">缓存创建</TableHead>
                  <TableHead className="w-40 text-right">缓存读取</TableHead>
                  {tab === "models" ? <TableHead className="w-28">状态</TableHead> : null}
                  <TableHead className="w-44 text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pagination.visibleItems.map((row) => {
                  const prices = modelPriceColumnValues(row.configured);
                  const tiers = row.configured.billing_expr
                    ? expressionPriceTiers(row.configured.billing_expr)
                    : [];
                  const showTiers = tiers.length > 1;
                  const comparisonStatus =
                    comparisonRequested && props.managementPrices
                      ? newAPIPriceComparisonStatus(row.configured, props.managementPrices)
                      : null;
                  const actionPending = selectionBusy;
                  const syncPending =
                    pendingModelAction?.model === row.model && pendingModelAction.kind === "sync";
                  return (
                    <TableRow key={row.model}>
                      {batchEnabled ? (
                        <TableCell className="align-top">
                          <PriceSelectionCheckbox
                            models={[row.model]}
                            selected={selectedModels}
                            label={`选择模型 ${row.model}`}
                            disabled={selectionBusy}
                            onChange={selectModels}
                          />
                        </TableCell>
                      ) : null}
                      <TableCell className="align-top font-mono text-xs font-medium">
                        <div
                          className={
                            showTiers
                              ? "grid grid-cols-[minmax(0,1fr)_minmax(7rem,10rem)] items-start gap-5"
                              : "flex min-h-5 items-baseline gap-2"
                          }
                        >
                          <div className="min-w-0">
                            <TableOverflowTooltip content={row.model}>
                              {row.model}
                            </TableOverflowTooltip>
                            <div className="text-muted-foreground mt-1 font-sans text-[11px] font-normal">
                              {showTiers
                                ? `阶梯计费 · ${tiers.length} 档`
                                : billingModeLabel(row.configured)}
                            </div>
                          </div>
                          {showTiers ? <TierLabels tiers={tiers} /> : null}
                        </div>
                      </TableCell>
                      <TableCell
                        className="align-top text-right font-mono text-xs"
                        aria-label={
                          showTiers
                            ? `${row.model} 输入价格：${tierPriceLabel(tiers, "input")}`
                            : `${row.model} 输入价格：${prices.input || "未设置"}`
                        }
                      >
                        {showTiers ? (
                          <TierPriceValues tiers={tiers} field="input" />
                        ) : (
                          prices.input || "-"
                        )}
                      </TableCell>
                      <TableCell
                        className="align-top text-right font-mono text-xs"
                        aria-label={
                          showTiers
                            ? `${row.model} 输出价格：${tierPriceLabel(tiers, "output")}`
                            : `${row.model} 输出价格：${prices.output || "未设置"}`
                        }
                      >
                        {showTiers ? (
                          <TierPriceValues tiers={tiers} field="output" />
                        ) : (
                          prices.output || "-"
                        )}
                      </TableCell>
                      <TableCell
                        className="align-top text-right font-mono text-xs"
                        aria-label={`${row.model} 缓存创建价格：${
                          showTiers
                            ? tierCacheCreatePriceLabel(tiers)
                            : cacheCreatePriceLabel(prices)
                        }`}
                      >
                        {showTiers ? (
                          <TierCacheCreatePrices tiers={tiers} />
                        ) : (
                          <CacheCreatePrices prices={prices} />
                        )}
                      </TableCell>
                      <TableCell
                        className="align-top text-right font-mono text-xs"
                        aria-label={
                          showTiers
                            ? `${row.model} 缓存读取价格：${tierPriceLabel(tiers, "cacheRead")}`
                            : `${row.model} 缓存读取价格：${prices.cacheRead || "未设置"}`
                        }
                      >
                        {showTiers ? (
                          <TierPriceValues tiers={tiers} field="cacheRead" />
                        ) : (
                          prices.cacheRead || "-"
                        )}
                      </TableCell>
                      {tab === "models" ? (
                        <TableCell className="align-top">
                          <ModelPriceComparisonStatus
                            configured={row.configured}
                            remotePrices={props.managementPrices}
                            requested={comparisonRequested}
                            pending={props.managementPricesPending ?? false}
                            error={props.managementPricesError ?? ""}
                            stale={props.managementPricesStale}
                          />
                        </TableCell>
                      ) : null}
                      <TableCell className="w-44 align-top text-right" overflowTooltip={false}>
                        {tab === "unset" ? (
                          <TableActionButton
                            label="查询远程价格"
                            ariaLabel={`查询 ${row.model} 远程模型价格`}
                            onClick={() => showRemotePrices(row.model)}
                          >
                            <Search aria-hidden="true" />
                          </TableActionButton>
                        ) : null}
                        {tab === "models" ? (
                          <div className="flex min-w-40 items-center justify-end gap-1">
                            {comparisonStatus === "mismatched" ? (
                              <TableActionButton
                                label="查看价格差异"
                                ariaLabel={`查看 ${row.model} 价格差异`}
                                onClick={() => {
                                  const remote = props.managementPrices
                                    ? matchingRemoteModelPrice(props.managementPrices, row.model)
                                    : null;
                                  if (remote) {
                                    setDifferenceSelection({ configured: row.configured, remote });
                                  }
                                }}
                              >
                                <GitCompareArrows aria-hidden="true" />
                              </TableActionButton>
                            ) : null}
                            {props.onWriteModelPrice || props.onSyncModelPrice ? (
                              <>
                                <TableActionButton
                                  label="上调价格"
                                  ariaLabel={`上调 ${row.model} 价格`}
                                  tone="primary"
                                  disabled={!props.onWriteModelPrice || actionPending}
                                  onClick={() =>
                                    setAdjustmentRequest({
                                      price: row.configured,
                                      direction: "increase",
                                    })
                                  }
                                >
                                  <TrendingUp aria-hidden="true" />
                                </TableActionButton>
                                <TableActionButton
                                  label="下调价格"
                                  ariaLabel={`下调 ${row.model} 价格`}
                                  disabled={!props.onWriteModelPrice || actionPending}
                                  onClick={() =>
                                    setAdjustmentRequest({
                                      price: row.configured,
                                      direction: "decrease",
                                    })
                                  }
                                >
                                  <TrendingDown aria-hidden="true" />
                                </TableActionButton>
                                <TableActionButton
                                  label="还原价格"
                                  ariaLabel={`还原 ${row.model} 价格`}
                                  disabled={!restorePrices[row.model] || actionPending}
                                  onClick={() => void restoreModelPrice(row.model)}
                                >
                                  <RotateCcw aria-hidden="true" />
                                </TableActionButton>
                                <TableActionButton
                                  label="同步远程价格"
                                  ariaLabel={`同步 ${row.model} 远程模型价格`}
                                  disabled={!props.onSyncModelPrice || actionPending}
                                  onClick={() => void syncModelPrice(row.configured)}
                                >
                                  <RefreshCw
                                    className={syncPending ? "animate-spin" : undefined}
                                    aria-hidden="true"
                                  />
                                </TableActionButton>
                              </>
                            ) : null}
                          </div>
                        ) : null}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
            <DataTablePagination
              currentPage={pagination.currentPage}
              totalPages={pagination.totalPages}
              totalItems={filteredRows.length}
              pageSize={pagination.pageSize}
              pageSizes={[10, 20, 50, 100]}
              onPageChange={pagination.setCurrentPage}
              onPageSizeChange={pagination.setPageSize}
            />
          </DataTablePanel>
        ) : null}
      </div>
      {batchEnabled ? (
        <ModelPriceSelectionToolbar
          selectedCount={selectedModels.size}
          pending={selectionBusy || batchPreview !== null}
          selectAllDisabled={
            tab === "remote" ? filteredRemotePrices.length === 0 : filteredRows.length === 0
          }
          onClear={() => setSelectedModels(new Set())}
          onSelectAll={() =>
            selectModels(
              tab === "remote"
                ? filteredRemotePrices.map((price) => price.model)
                : filteredRows.map((row) => row.model),
              true,
            )
          }
          onSync={() => void prepareBatchSync()}
        />
      ) : null}
      <ModelPriceDifferenceDialog
        selection={differenceSelection}
        onOpenChange={(open) => {
          if (!open) setDifferenceSelection(null);
        }}
      />
      <BatchModelPriceDialog
        preview={batchPreview}
        selectedCount={batchPreview?.length || selectedModels.size}
        preparing={batchPreparing}
        writing={batchWriting}
        failed={batchFailed}
        results={batchResults}
        onClose={closeBatchPreview}
        onRetry={() => void prepareBatchSync()}
        onConfirm={() => void confirmBatchSync()}
      />
      <WrittenModelPriceDialog
        price={props.writtenManagementPrice ?? null}
        onOpenChange={(open) => props.onWrittenManagementPriceOpenChange?.(open)}
      />
      {adjustmentRequest ? (
        <ModelPriceAdjustmentDialog
          key={`${adjustmentRequest.price.model}-${adjustmentRequest.direction}`}
          request={adjustmentRequest}
          pending={pendingModelAction?.model === adjustmentRequest.price.model}
          onOpenChange={(open) => {
            if (!open && pendingModelAction?.model !== adjustmentRequest.price.model) {
              setAdjustmentRequest(null);
            }
          }}
          onSubmit={(values) => void submitAdjustment(adjustmentRequest, values)}
        />
      ) : null}
    </div>
  );
}

function ModelPriceAdjustmentDialog(props: {
  request: PriceAdjustmentRequest;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: ModelPriceAdjustmentValues) => void;
}) {
  const form = useForm<ModelPriceAdjustmentValues>({
    resolver: zodResolver(modelPriceAdjustmentSchema(props.request.direction)),
    defaultValues: { percentage: 10 },
  });
  const operation = props.request.direction === "increase" ? "上调" : "下调";

  return (
    <Dialog open onOpenChange={props.onOpenChange}>
      <DialogContent showCloseButton={!props.pending}>
        <DialogHeader>
          <DialogTitle>{`${props.request.price.model} 价格${operation}`}</DialogTitle>
          <DialogDescription>
            按当前平台价格整体{operation}，相对输出与缓存倍率保持不变。
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <form
            id="model-price-adjustment-form"
            className="grid gap-1.5"
            onSubmit={form.handleSubmit(props.onSubmit)}
          >
            <label htmlFor="model-price-adjustment-percentage" className="text-sm font-medium">
              调整百分比
            </label>
            <div className="relative">
              <Input
                id="model-price-adjustment-percentage"
                className="pr-9"
                type="number"
                inputMode="decimal"
                min="0.01"
                max={props.request.direction === "decrease" ? "99.99" : "1000"}
                step="0.01"
                aria-invalid={Boolean(form.formState.errors.percentage)}
                disabled={props.pending}
                {...form.register("percentage", { valueAsNumber: true })}
              />
              <span className="text-muted-foreground pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm">
                %
              </span>
            </div>
            {form.formState.errors.percentage?.message ? (
              <span className="text-destructive text-xs" role="alert">
                {form.formState.errors.percentage.message}
              </span>
            ) : null}
          </form>
        </DialogBody>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={props.pending}
            onClick={() => props.onOpenChange(false)}
          >
            取消
          </Button>
          <Button type="submit" form="model-price-adjustment-form" disabled={props.pending}>
            {props.pending ? "正在写入" : `确认${operation}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function RemoteModelPricesTable(props: {
  prices: Sub2APIModelPrice[];
  pending: boolean;
  error: string;
  filtered?: boolean;
  writingModel?: string;
  onWritePrice?: (price: Sub2APIModelPrice) => void;
  selectedModels?: ReadonlySet<string>;
  onSelectModels?: (models: string[], checked: boolean) => void;
  selectionDisabled?: boolean;
}) {
  const pagination = useClientPagination(props.prices, 10);

  return (
    <DataTablePanel className="flex min-h-0 flex-1 flex-col">
      {props.pending && props.prices.length === 0 && (
        <PageLoadingSkeleton label="正在获取远程模型价格" />
      )}
      {!props.pending && !props.error && props.prices.length === 0 && (
        <div className="text-muted-foreground grid min-h-52 place-items-center px-6 text-center text-sm">
          {props.filtered ? "没有匹配的模型" : "远程价卡未返回模型价格"}
        </div>
      )}
      {props.prices.length > 0 && (
        <Table
          containerClassName="min-h-0 flex-1 overflow-auto"
          overflowTooltip={false}
          className="min-w-[64rem]"
        >
          <TableHeader>
            <TableRow>
              {props.selectedModels && props.onSelectModels ? (
                <TableHead className="w-10">
                  <PriceSelectionCheckbox
                    models={pagination.visibleItems.map((price) => price.model)}
                    selected={props.selectedModels}
                    label="选择本页参考模型"
                    disabled={props.selectionDisabled}
                    onChange={props.onSelectModels}
                  />
                </TableHead>
              ) : null}
              <TableHead className="min-w-56">模型</TableHead>
              <TableHead className="text-right">输入价格（每百万 Token）</TableHead>
              <TableHead className="text-right">输出价格（每百万 Token）</TableHead>
              <TableHead className="text-right">缓存写入</TableHead>
              <TableHead className="text-right">缓存读取</TableHead>
              <TableHead className="text-right">图片价格</TableHead>
              {props.onWritePrice ? <TableHead className="w-36 text-right">操作</TableHead> : null}
            </TableRow>
          </TableHeader>
          <TableBody>
            {pagination.visibleItems.map((price) => {
              const writeSupported = remotePriceSupportsNewAPIWrite(price);
              return (
                <TableRow key={price.model}>
                  {props.selectedModels && props.onSelectModels ? (
                    <TableCell className="align-top">
                      <PriceSelectionCheckbox
                        models={[price.model]}
                        selected={props.selectedModels}
                        label={`选择参考模型 ${price.model}`}
                        disabled={props.selectionDisabled}
                        onChange={props.onSelectModels}
                      />
                    </TableCell>
                  ) : null}
                  <TableCell className="font-mono text-xs font-medium">
                    <div>{price.model}</div>
                    <div className="text-muted-foreground mt-1 font-sans text-[11px] font-normal">
                      {price.source === "official" && price.source_url ? (
                        <a
                          href={price.source_url}
                          target="_blank"
                          rel="noreferrer"
                          className="underline underline-offset-2"
                        >
                          {modelPriceSourceLabels.official}
                        </a>
                      ) : (
                        modelPriceSourceLabels[price.source ?? "remote"]
                      )}
                    </div>
                    {price.source_scope ? (
                      <div className="text-muted-foreground mt-1 max-w-80 whitespace-normal font-sans text-[11px] font-normal">
                        {price.source_scope}
                      </div>
                    ) : null}
                    {price.time_pricing ? (
                      <div className="text-muted-foreground mt-1 max-w-80 whitespace-normal font-sans text-[11px] font-normal">
                        {timePricingDescription(price.time_pricing)}
                      </div>
                    ) : null}
                    {price.long_context_threshold ? (
                      <div className="text-muted-foreground mt-1 font-sans text-[11px] font-normal">
                        阶梯 {formatRemoteThreshold(price.long_context_threshold)}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    <OfficialTierValues tiers={price.price_tiers} field="input_price">
                      {price.time_pricing ? (
                        <TimePriceValues
                          peak={price.time_pricing.peak.input_price}
                          offPeak={price.input_price}
                        />
                      ) : (
                        managementTierPrice(price.input_price, price.long_context_input_price)
                      )}
                    </OfficialTierValues>
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    <OfficialTierValues tiers={price.price_tiers} field="output_price">
                      {price.time_pricing ? (
                        <TimePriceValues
                          peak={price.time_pricing.peak.output_price}
                          offPeak={price.output_price}
                        />
                      ) : (
                        managementTierPrice(price.output_price, price.long_context_output_price)
                      )}
                    </OfficialTierValues>
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    <OfficialTierValues tiers={price.price_tiers} field="cache_write_price">
                      <ManagementCacheWritePrice price={price} />
                    </OfficialTierValues>
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    <OfficialTierValues tiers={price.price_tiers} field="cache_read_price">
                      {price.time_pricing ? (
                        <TimePriceValues
                          peak={price.time_pricing.peak.cache_read_price}
                          offPeak={price.cache_read_price ?? "0"}
                        />
                      ) : (
                        managementTierPrice(
                          price.cache_read_price,
                          price.long_context_cache_read_price,
                        )
                      )}
                    </OfficialTierValues>
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    <ManagementImagePrice price={price} />
                  </TableCell>
                  {props.onWritePrice ? (
                    <TableCell className="text-right" overflowTooltip={false}>
                      <RemotePriceWriteAction
                        price={price}
                        writeSupported={writeSupported}
                        writingModel={props.writingModel}
                        onWritePrice={props.onWritePrice}
                      />
                    </TableCell>
                  ) : null}
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
      {props.prices.length > 0 ? (
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={props.prices.length}
          pageSize={pagination.pageSize}
          pageSizes={[10, 20, 50, 100]}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      ) : null}
    </DataTablePanel>
  );
}

export function filterRemoteModelPrices(
  prices: Sub2APIModelPrice[],
  search: string,
): Sub2APIModelPrice[] {
  const query = search.trim().toLocaleLowerCase();
  return [...prices]
    .sort((left, right) => left.model.localeCompare(right.model))
    .filter((price) => !query || price.model.toLocaleLowerCase().includes(query));
}

export function matchingRemoteModelPrice(
  prices: Sub2APIModelPrice[],
  model: string,
): Sub2APIModelPrice | null {
  const exact = prices.find((price) => price.model === model);
  if (exact) return exact;

  const thinkingVariant = /^(gemini-.+)-(high|low|medium|tiered)$/.exec(model);
  if (!thinkingVariant) return null;
  const base = prices.find((price) => price.model === thinkingVariant[1]);
  return base ? { ...base, model } : null;
}

export function remotePriceToNewAPIModelPrice(price: Sub2APIModelPrice): NewAPIModelPrice {
  const result: NewAPIModelPrice = {
    model: price.model,
    input_ratio: price.model_ratio,
    completion_ratio: price.completion_ratio,
    ...(price.cache_ratio ? { cache_ratio: price.cache_ratio } : {}),
    ...(price.create_cache_ratio ? { create_cache_ratio: price.create_cache_ratio } : {}),
    ...(price.image_ratio ? { image_ratio: price.image_ratio } : {}),
  };
  const billingExpression = remotePriceBillingExpression(price);
  if (billingExpression) {
    result.billing_mode = "tiered_expr";
    result.billing_expr = billingExpression;
  }
  return result;
}

function remotePriceSupportsNewAPIWrite(price: Sub2APIModelPrice): boolean {
  const inputRatio = Number(price.model_ratio);
  const completionRatio = Number(price.completion_ratio);
  return (
    price.model_ratio.trim() !== "" &&
    price.completion_ratio.trim() !== "" &&
    Number.isFinite(inputRatio) &&
    inputRatio >= 0 &&
    Number.isFinite(completionRatio) &&
    completionRatio >= 0
  );
}

function RemotePriceWriteAction(props: {
  price: Sub2APIModelPrice;
  writeSupported: boolean;
  writingModel?: string;
  onWritePrice: (price: Sub2APIModelPrice) => void;
}) {
  if (!props.writeSupported) {
    return (
      <TableActionButton
        label="暂不支持写入"
        ariaLabel={`暂不支持写入 ${props.price.model}`}
        disabled
      >
        <Upload aria-hidden="true" />
      </TableActionButton>
    );
  }
  if (props.writingModel === props.price.model) {
    return (
      <TableActionButton label="正在写入" ariaLabel={`正在写入 ${props.price.model}`} disabled>
        <RefreshCw className="animate-spin" aria-hidden="true" />
      </TableActionButton>
    );
  }
  return (
    <TableActionButton
      label="写入平台"
      ariaLabel={`写入平台 ${props.price.model}`}
      tone="primary"
      disabled={props.writingModel !== undefined}
      onClick={() => props.onWritePrice(props.price)}
    >
      <Upload aria-hidden="true" />
    </TableActionButton>
  );
}

function remotePriceBillingExpression(price: Sub2APIModelPrice): string {
  if (price.billing_expr) return price.billing_expr;
  if (price.time_pricing) return timePricingExpression(price);
  if (price.long_context_threshold) {
    const standard = remotePriceTierExpression(price, false);
    const longContext = remotePriceTierExpression(price, true);
    if (!standard || !longContext) return "";
    const operator = price.long_context_threshold_inclusive ? "<" : "<=";
    return `len ${operator} ${price.long_context_threshold} ? tier("standard", ${standard}) : tier("long_context", ${longContext})`;
  }
  if (!price.cache_write_1h_price?.trim()) return "";
  const expression = remotePriceTierExpression(price, false);
  return expression ? `tier("base", ${expression})` : "";
}

function remotePriceTierExpression(price: Sub2APIModelPrice, longContext: boolean): string {
  const candidates: Array<[string, string | undefined]> = [
    ["p", longContext ? price.long_context_input_price : price.input_price],
    ["c", longContext ? price.long_context_output_price : price.output_price],
    ["cr", longContext ? price.long_context_cache_read_price : price.cache_read_price],
    ["cc", longContext ? price.long_context_cache_write_price : price.cache_write_price],
    ["cc1h", longContext ? price.long_context_cache_write_1h_price : price.cache_write_1h_price],
    ["img", price.image_input_price],
  ];
  const terms = candidates.flatMap(([variable, rawPrice]) => {
    const value = pricePerMillion(rawPrice);
    return value ? [`${variable} * ${value}`] : [];
  });
  return terms.join(" + ");
}

export type NewAPIPriceComparisonStatus = "matched" | "mismatched" | "missing";

export function newAPIPriceComparisonStatus(
  configured: NewAPIModelPrice,
  remotePrices: Sub2APIModelPrice[],
): NewAPIPriceComparisonStatus {
  const remote = matchingRemoteModelPrice(remotePrices, configured.model);
  if (!remote) return "missing";
  const expected = remotePriceToNewAPIModelPrice(remote);
  if (remote.time_pricing || remote.billing_expr) {
    return configured.billing_mode === "tiered_expr" &&
      configured.billing_expr?.replace(/\s+/g, "") === expected.billing_expr?.replace(/\s+/g, "")
      ? "matched"
      : "mismatched";
  }
  if (expected.billing_mode === "tiered_expr") {
    if (configured.billing_mode !== "tiered_expr" || !configured.billing_expr?.trim()) {
      return "mismatched";
    }
    if (!tieredExpressionStructureMatches(configured.billing_expr, expected.billing_expr ?? "")) {
      return "mismatched";
    }
    const configuredPrices = modelPriceColumnValues(configured);
    const expectedPrices = comparisonColumnValues(configuredPrices, remote);
    const fields: Array<keyof ModelPriceColumnValues> = [
      "input",
      "output",
      "cacheCreate",
      "cacheCreate1h",
      "cacheRead",
      "imageInput",
    ];
    return fields.every((field) =>
      decimalValuesEqual(configuredPrices[field], expectedPrices[field]),
    )
      ? "matched"
      : "mismatched";
  }
  if (
    configured.model_price?.trim() ||
    configured.billing_expr?.trim() ||
    (configured.billing_mode && configured.billing_mode !== "per-token")
  ) {
    return "mismatched";
  }

  const configuredPrices = modelPriceColumnValues(configured);
  const expectedPrices = comparisonColumnValues(configuredPrices, remote);
  const fields: Array<keyof ModelPriceColumnValues> = [
    "input",
    "output",
    "cacheCreate",
    "cacheCreate1h",
    "cacheRead",
    "imageInput",
  ];
  const matched = fields.every((field) =>
    decimalValuesEqual(configuredPrices[field], expectedPrices[field]),
  );
  return matched ? "matched" : "mismatched";
}

function decimalValuesEqual(left: string | undefined, right: string | undefined): boolean {
  return modelPriceNumbersEqual(left, right);
}

function comparisonColumnValues(
  configured: ModelPriceColumnValues,
  remote: Sub2APIModelPrice,
): ModelPriceColumnValues {
  const expected = modelPriceColumnValues(remotePriceToNewAPIModelPrice(remote));
  if (remote.source !== "sub2api") return expected;
  for (const field of ["cacheCreate", "cacheCreate1h", "imageInput"] as const) {
    if (!configured[field] && decimalValuesEqual(expected[field], "0")) expected[field] = "";
  }
  return expected;
}

function ModelPriceComparisonStatus(props: {
  configured: NewAPIModelPrice;
  remotePrices?: Sub2APIModelPrice[];
  requested: boolean;
  pending: boolean;
  error: string;
  stale?: boolean;
}) {
  if (!props.requested) return <StatusBadge label="未比较" variant="neutral" />;
  if (props.error && props.remotePrices === undefined)
    return <StatusBadge label="比较失败" variant="danger" />;
  if (props.remotePrices === undefined) {
    return <StatusBadge label="比较中" variant="info" pulse />;
  }

  const status = newAPIPriceComparisonStatus(props.configured, props.remotePrices);
  if (status === "matched") return <StatusBadge label="一致" variant="success" />;
  if (status === "missing")
    return <StatusBadge label={props.stale ? "价格待确认" : "参考价未找到"} variant="neutral" />;
  return <StatusBadge label="不一致" variant="warning" />;
}

export type ModelPriceDifferenceRow = {
  label: string;
  configured: string;
  remote: string;
  matched: boolean;
};

export function modelPriceDifferenceRows(
  configured: NewAPIModelPrice,
  remote: Sub2APIModelPrice,
): ModelPriceDifferenceRow[] {
  const configuredPrices = modelPriceColumnValues(configured);
  const expected = remotePriceToNewAPIModelPrice(remote);
  const remotePrices = comparisonColumnValues(configuredPrices, remote);
  const configuredMode = billingModeLabel(configured);
  const remoteMode = billingModeLabel(expected);
  const rows: Array<{
    label: string;
    configured: string;
    remote: string;
    kind: "text" | "decimal";
    optional?: boolean;
  }> = [
    { label: "计费方式", configured: configuredMode, remote: remoteMode, kind: "text" },
    {
      label: "输入价格（$/百万 Token）",
      configured: configuredPrices.input,
      remote: remotePrices.input,
      kind: "decimal",
    },
    {
      label: "输出价格（$/百万 Token）",
      configured: configuredPrices.output,
      remote: remotePrices.output,
      kind: "decimal",
    },
    {
      label: "缓存写入（$/百万 Token）",
      optional: true,
      configured: configuredPrices.cacheCreate,
      remote: remotePrices.cacheCreate,
      kind: "decimal",
    },
    {
      label: "缓存写入（1h）（$/百万 Token）",
      optional: true,
      configured: configuredPrices.cacheCreate1h,
      remote: remotePrices.cacheCreate1h,
      kind: "decimal",
    },
    {
      label: "缓存读取（$/百万 Token）",
      configured: configuredPrices.cacheRead,
      remote: remotePrices.cacheRead,
      kind: "decimal",
    },
    {
      label: "图片输入（$/百万 Token）",
      optional: true,
      configured: configuredPrices.imageInput,
      remote: remotePrices.imageInput,
      kind: "decimal",
    },
  ];
  const configuredCondition = tierConditionSignature(configured.billing_expr ?? "");
  const remoteCondition = tierConditionSignature(expected.billing_expr ?? "");
  if (remote.time_pricing) {
    const description = timePricingDescription(remote.time_pricing);
    rows.splice(1, 0, {
      label: "峰谷时段",
      configured: configuredCondition === remoteCondition ? description : "未配置或与官方时段不同",
      remote: description,
      kind: "text",
    });
    for (const row of rows) {
      if (row.kind === "decimal" && row.remote.includes(" / ")) row.label += "（高峰 / 空闲）";
    }
  } else if (remote.price_tiers?.length) {
    const description = remote.price_tiers.map((tier) => tier.label).join(" / ");
    rows.splice(1, 0, {
      label: "官方阶梯",
      configured:
        configured.billing_expr?.replace(/\s+/g, "") === remote.billing_expr?.replace(/\s+/g, "")
          ? description
          : "未配置或与官方条件不同",
      remote: description,
      kind: "text",
    });
  } else if (configuredCondition || remoteCondition) {
    rows.splice(1, 0, {
      label: "阶梯条件",
      configured: configuredCondition,
      remote: remoteCondition,
      kind: "text",
    });
  }
  return rows
    .filter((row) => !row.optional || row.configured !== "" || row.remote !== "")
    .map((row) => ({
      label: row.label,
      configured: row.configured || "-",
      remote: row.remote || "-",
      matched:
        row.kind === "text"
          ? row.configured === row.remote
          : decimalValuesEqual(row.configured, row.remote),
    }));
}

function tierConditionSignature(expression: string): string {
  if (/\b(?:hour|minute|weekday)\(/.test(expression))
    return expression.split("?")[0].replace(/\s+/g, "");
  const match = /\blen\s*(<=|<|>=|>)\s*(\d+(?:\.\d+)?)/.exec(expression);
  if (!match?.[1] || !match[2]) return "";
  return `len ${match[1]} ${match[2]}`;
}

function tieredExpressionStructureMatches(configured: string, expected: string): boolean {
  const configuredTiers = expressionPriceTiers(configured);
  const expectedTiers = expressionPriceTiers(expected);
  if (configuredTiers.length !== expectedTiers.length) return false;
  return tierConditionSignature(configured) === tierConditionSignature(expected);
}

function ModelPriceDifferenceDialog(props: {
  selection: PriceDifferenceSelection | null;
  onOpenChange: (open: boolean) => void;
}) {
  const rows = props.selection
    ? modelPriceDifferenceRows(props.selection.configured, props.selection.remote)
    : [];
  return (
    <Dialog open={props.selection !== null} onOpenChange={props.onOpenChange}>
      <DialogContent width="wide" height="adaptive">
        <DialogHeader>
          <DialogTitle>{props.selection?.configured.model ?? "模型价格"}</DialogTitle>
          <DialogDescription>当前平台配置与远程价卡的价格对照。</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <Table overflowTooltip={false}>
            <TableHeader>
              <TableRow>
                <TableHead>价格项</TableHead>
                <TableHead className="text-right">当前平台</TableHead>
                <TableHead className="text-right">远程价格</TableHead>
                <TableHead className="w-24 text-right">结果</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={row.label}>
                  <TableCell>{row.label}</TableCell>
                  <TableCell className="text-right font-mono text-xs">{row.configured}</TableCell>
                  <TableCell className="text-right font-mono text-xs">{row.remote}</TableCell>
                  <TableCell className="text-right">
                    <StatusBadge
                      label={row.matched ? "一致" : "不一致"}
                      variant={row.matched ? "success" : "warning"}
                    />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

function WrittenModelPriceDialog(props: {
  price: NewAPIModelPrice | null;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={props.price !== null} onOpenChange={props.onOpenChange}>
      <DialogContent width="wide" height="adaptive">
        <DialogHeader>
          <DialogTitle>{props.price ? `${props.price.model} 写入结果` : "写入结果"}</DialogTitle>
          <DialogDescription>写入平台后重新读取到的实际配置。</DialogDescription>
        </DialogHeader>
        <DialogBody>
          {props.price ? <WrittenModelPriceResult price={props.price} /> : null}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

export function WrittenModelPriceResult(props: { price: NewAPIModelPrice }) {
  const prices = modelPriceColumnValues(props.price);
  const rows = [
    { label: "计费方式", value: billingModeLabel(props.price) },
    { label: "输入价格", value: prices.input },
    { label: "输出价格", value: prices.output },
    { label: "缓存读取", value: prices.cacheRead },
    { label: "缓存写入", value: prices.cacheCreate },
    { label: "缓存写入（1h）", value: prices.cacheCreate1h },
    { label: "图片输入", value: prices.imageInput },
    { label: "图片输出", value: prices.imageOutput },
    { label: "音频输入", value: prices.audioInput },
    { label: "音频输出", value: prices.audioOutput },
  ].filter((row) => row.label === "计费方式" || row.value);
  return (
    <div className="grid gap-4">
      <Table overflowTooltip={false}>
        <TableHeader>
          <TableRow>
            <TableHead>价格项</TableHead>
            <TableHead className="text-right">平台读回结果</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={row.label}>
              <TableCell>{row.label}</TableCell>
              <TableCell className="text-right font-mono text-xs">{row.value}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {props.price.billing_expr ? (
        <div className="grid gap-1.5">
          <div className="text-muted-foreground text-xs">计费表达式</div>
          <code className="overflow-x-auto rounded-md border bg-muted/30 p-3 font-mono text-xs">
            {props.price.billing_expr}
          </code>
        </div>
      ) : null}
    </div>
  );
}

function managementPricePerMillion(value?: string): string {
  return pricePerMillion(value) || "-";
}

function managementTierPrice(base?: string, longContext?: string): string {
  const basePrice = managementPricePerMillion(base);
  const longContextPrice = managementPricePerMillion(longContext);
  if (longContextPrice === "-") return basePrice;
  return `${basePrice} / ${longContextPrice}`;
}

function formatRemoteThreshold(threshold: number): string {
  if (threshold >= 1000 && threshold % 1000 === 0) return `${threshold / 1000}K`;
  return String(threshold);
}

function ManagementCacheWritePrice(props: { price: Sub2APIModelPrice }): ReactNode {
  const normal = managementTierPrice(
    props.price.cache_write_price,
    props.price.long_context_cache_write_price,
  );
  const oneHour = managementTierPrice(
    props.price.cache_write_1h_price,
    props.price.long_context_cache_write_1h_price,
  );
  if (normal === "-" && oneHour === "-") return "-";
  if (oneHour === "-") return normal;
  return (
    <span className="whitespace-nowrap">
      {normal !== "-" ? <>普通 {normal} </> : null}
      <span className="text-muted-foreground font-sans text-[11px]">1 小时</span> {oneHour}
    </span>
  );
}

function ManagementImagePrice(props: { price: Sub2APIModelPrice }): ReactNode {
  const input = managementPricePerMillion(props.price.image_input_price);
  const output = managementPriceValue(props.price.image_output_price);
  if (input === "-" && output === "-") return "-";
  return (
    <span className="whitespace-nowrap">
      {input !== "-" ? <>输入 {input}</> : null}
      {input !== "-" && output !== "-" ? "，" : null}
      {output !== "-" ? <>输出 {output}</> : null}
    </span>
  );
}

function managementPriceValue(value?: string): string {
  return formatModelPriceNumber(value) || "-";
}

function ratioPrice(ratio?: string): string {
  if (!ratio?.trim()) return "";
  const value = Number(ratio);
  return Number.isFinite(value) && value >= 0 ? formatModelPriceNumber(value * 2) : "";
}

function inputPriceValue(price: NewAPIModelPrice): string {
  return formatModelPriceNumber(price.input_price) || ratioPrice(price.input_ratio);
}

function outputPriceValue(price: NewAPIModelPrice): string {
  if (price.completion_price) return formatModelPriceNumber(price.completion_price);
  const inputPrice = inputPriceValue(price);
  if (!inputPrice || !price.completion_ratio?.trim()) return "";
  const input = Number(inputPrice);
  const completion = Number(price.completion_ratio);
  return Number.isFinite(input) && Number.isFinite(completion)
    ? formatModelPriceNumber(input * completion)
    : "";
}

function cachePriceValue(
  ratio: string | undefined,
  explicit: string | undefined,
  model: NewAPIModelPrice,
): string {
  if (explicit) return formatModelPriceNumber(explicit);
  const inputPrice = inputPriceValue(model);
  if (!inputPrice || !ratio?.trim()) return "";
  const input = Number(inputPrice);
  const cache = Number(ratio);
  return Number.isFinite(input) && Number.isFinite(cache)
    ? formatModelPriceNumber(input * cache)
    : "";
}

type ModelPriceColumnValues = {
  input: string;
  output: string;
  cacheCreate: string;
  cacheCreate1h: string;
  cacheRead: string;
  imageInput: string;
  imageOutput: string;
  audioInput: string;
  audioOutput: string;
};

type ExpressionPriceTier = ModelPriceColumnValues & {
  label: string;
};

type BillingExpressionVariable = "p" | "c" | "cr" | "cc" | "cc1h" | "img" | "img_o" | "ai" | "ao";

function expressionPriceValue(expression: string, variable: BillingExpressionVariable): string {
  const numberPattern = "-?(?:\\d+(?:\\.\\d*)?|\\.\\d+)(?:[eE][+-]?\\d+)?";
  const pattern = new RegExp(`\\b${variable}\\s*\\*\\s*(${numberPattern})`, "g");
  const values: string[] = [];
  for (const match of expression.matchAll(pattern)) {
    const value = match[1];
    const formatted = formatModelPriceNumber(value);
    if (formatted && !values.includes(formatted)) values.push(formatted);
  }
  return values.join(" / ");
}

function expressionPriceTiers(expression: string): ExpressionPriceTier[] {
  const tierPattern = /tier\(\s*"([^"]*)"\s*,\s*([^)]+)\)/g;
  const tiers: ExpressionPriceTier[] = [];
  for (const match of expression.matchAll(tierPattern)) {
    const label = match[1];
    const body = match[2];
    if (label === undefined || body === undefined) continue;
    tiers.push({
      label: label || `档位 ${tiers.length + 1}`,
      input: expressionPriceValue(body, "p"),
      output: expressionPriceValue(body, "c"),
      cacheCreate: expressionPriceValue(body, "cc"),
      cacheCreate1h: expressionPriceValue(body, "cc1h"),
      cacheRead: expressionPriceValue(body, "cr"),
      imageInput: expressionPriceValue(body, "img"),
      imageOutput: expressionPriceValue(body, "img_o"),
      audioInput: expressionPriceValue(body, "ai"),
      audioOutput: expressionPriceValue(body, "ao"),
    });
  }
  return tiers;
}

function cacheCreatePriceLabel(prices: ModelPriceColumnValues): string {
  const values: string[] = [];
  if (prices.cacheCreate) values.push(`普通 ${prices.cacheCreate}`);
  if (prices.cacheCreate1h) values.push(`1 小时 ${prices.cacheCreate1h}`);
  return values.length > 0 ? values.join("，") : "未设置";
}

function CacheCreatePrices(props: { prices: ModelPriceColumnValues }) {
  if (!props.prices.cacheCreate1h) return props.prices.cacheCreate || "-";
  return (
    <div className="flex flex-col gap-0.5 whitespace-nowrap">
      {props.prices.cacheCreate ? (
        <span>
          <span className="text-muted-foreground">普通</span> {props.prices.cacheCreate}
        </span>
      ) : null}
      <span>
        <span className="text-muted-foreground">1 小时</span> {props.prices.cacheCreate1h}
      </span>
    </div>
  );
}

type TierPriceField = "input" | "output" | "cacheRead";

function TierPriceValues(props: { tiers: ExpressionPriceTier[]; field: TierPriceField }) {
  return (
    <div
      className="flex min-w-28 flex-col gap-1.5 text-right whitespace-nowrap"
      data-slot="tier-price-values"
    >
      {props.tiers.map((tier) => (
        <span key={tier.label} className="block min-h-5">
          {tier[props.field] || "-"}
        </span>
      ))}
    </div>
  );
}

function TierCacheCreatePrices(props: { tiers: ExpressionPriceTier[] }) {
  return (
    <div className="flex min-w-28 flex-col gap-1.5" data-slot="tier-price-values">
      {props.tiers.map((tier) => (
        <div
          key={tier.label}
          className="flex min-h-5 items-baseline justify-end gap-2 whitespace-nowrap"
        >
          {tier.cacheCreate && tier.cacheCreate1h ? (
            <span>
              <span className="text-muted-foreground font-sans text-[11px]">普通</span>{" "}
              {tier.cacheCreate}
            </span>
          ) : null}
          {tier.cacheCreate && !tier.cacheCreate1h ? tier.cacheCreate : null}
          {tier.cacheCreate1h ? (
            <span>
              <span className="text-muted-foreground font-sans text-[11px]">1 小时</span>{" "}
              {tier.cacheCreate1h}
            </span>
          ) : null}
          {!tier.cacheCreate && !tier.cacheCreate1h ? "-" : null}
        </div>
      ))}
    </div>
  );
}

function TierLabels(props: { tiers: ExpressionPriceTier[] }) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5" data-slot="tier-labels">
      {props.tiers.map((tier) => (
        <TableOverflowTooltip
          key={tier.label}
          className="text-muted-foreground block min-h-5 truncate border-l-2 border-border pl-2 font-mono text-[11px] font-normal"
          content={tier.label}
        >
          {tier.label}
        </TableOverflowTooltip>
      ))}
    </div>
  );
}

function tierPriceLabel(tiers: ExpressionPriceTier[], field: TierPriceField): string {
  return tiers.map((tier) => `${tier.label} ${tier[field] || "未设置"}`).join("，");
}

function tierCacheCreatePriceLabel(tiers: ExpressionPriceTier[]): string {
  return tiers.map((tier) => `${tier.label} ${cacheCreatePriceLabel(tier)}`).join("，");
}

export function modelPriceColumnValues(price: NewAPIModelPrice): ModelPriceColumnValues {
  if (price.billing_mode === "tiered_expr" && price.billing_expr) {
    return {
      input: expressionPriceValue(price.billing_expr, "p"),
      output: expressionPriceValue(price.billing_expr, "c"),
      cacheCreate: expressionPriceValue(price.billing_expr, "cc"),
      cacheCreate1h: expressionPriceValue(price.billing_expr, "cc1h"),
      cacheRead: expressionPriceValue(price.billing_expr, "cr"),
      imageInput: expressionPriceValue(price.billing_expr, "img"),
      imageOutput: expressionPriceValue(price.billing_expr, "img_o"),
      audioInput: expressionPriceValue(price.billing_expr, "ai"),
      audioOutput: expressionPriceValue(price.billing_expr, "ao"),
    };
  }
  if (price.model_price?.trim()) {
    return {
      input: formatModelPriceNumber(price.model_price),
      output: "",
      cacheCreate: "",
      cacheCreate1h: "",
      cacheRead: "",
      imageInput: "",
      imageOutput: "",
      audioInput: "",
      audioOutput: "",
    };
  }
  const audioInput = cachePriceValue(price.audio_ratio, undefined, price);
  return {
    input: inputPriceValue(price),
    output: outputPriceValue(price),
    cacheCreate: cachePriceValue(price.create_cache_ratio, price.cache_create_price, price),
    cacheCreate1h: cachePriceValue(price.create_cache_1h_ratio, undefined, price),
    cacheRead: cachePriceValue(price.cache_ratio, price.cache_read_price, price),
    imageInput: cachePriceValue(price.image_ratio, undefined, price),
    imageOutput: "",
    audioInput,
    audioOutput: multiplyPrice(audioInput, price.audio_completion_ratio),
  };
}

function multiplyPrice(basePrice: string, ratio?: string): string {
  if (!basePrice || !ratio?.trim()) return "";
  const base = Number(basePrice);
  const multiplier = Number(ratio);
  return Number.isFinite(base) && Number.isFinite(multiplier)
    ? formatModelPriceNumber(base * multiplier)
    : "";
}

function billingModeLabel(price: NewAPIModelPrice | null): string {
  if (!price) return "-";
  if (price.billing_mode === "per-request") return "按次";
  if (price.billing_mode === "per-second" || price.billing_mode === "per_second") return "按秒";
  if (price.billing_mode === "tiered_expr") return "阶梯";
  if (price.model_price) return "固定价格";
  return "按 Token";
}

type PriceDifference = NewAPIRemoteSnapshot["differences"][number];

export function compareConfiguredWithModelPlaza(
  configuredModels: NewAPIModelPrice[],
  modelPlazaModels: NewAPIModelPrice[],
): PriceDifference[] {
  const fields: Array<keyof NewAPIModelPrice> = [
    "model_price",
    "input_price",
    "completion_price",
    "cache_create_price",
    "cache_read_price",
    "billing_mode",
    "billing_expr",
    "input_ratio",
    "completion_ratio",
    "cache_ratio",
    "create_cache_ratio",
    "create_cache_1h_ratio",
    "image_ratio",
    "audio_ratio",
    "audio_completion_ratio",
  ];
  const modelPlazaByModel = new Map(modelPlazaModels.map((item) => [item.model, item]));
  return configuredModels
    .flatMap((configured): PriceDifference[] => {
      const reference = modelPlazaByModel.get(configured.model) ?? null;
      if (!reference) {
        return [{ model: configured.model, kind: "missing_in_model_plaza", configured, reference }];
      }
      if (fields.some((field) => (configured[field] ?? "") !== (reference[field] ?? ""))) {
        return [{ model: configured.model, kind: "ratio_mismatch", configured, reference }];
      }
      return [];
    })
    .sort((left, right) => left.model.localeCompare(right.model));
}

export function NewAPIPriceDifferences(props: { snapshot: NewAPIRemoteSnapshot }) {
  const rows = useMemo(() => {
    if (props.snapshot.differences.length === 0) {
      return [...props.snapshot.models].sort((left, right) =>
        left.model.localeCompare(right.model),
      );
    }
    const configuredByModel = new Map(props.snapshot.models.map((model) => [model.model, model]));
    return props.snapshot.differences
      .map((difference) => {
        const configured = configuredByModel.get(difference.model);
        if (configured && difference.configured) {
          return { ...configured, ...difference.configured };
        }
        return difference.configured ?? configured ?? difference.reference;
      })
      .filter((value): value is NewAPIModelPrice => value !== null)
      .sort((left, right) => left.model.localeCompare(right.model));
  }, [props.snapshot.differences, props.snapshot.models]);
  const pagination = useClientPagination(rows);
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      {rows.length === 0 ? (
        <div className="text-muted-foreground flex min-h-52 items-center justify-center px-6 text-sm">
          尚未读取到本平台模型价格
        </div>
      ) : (
        <DataTablePanel className="flex-1">
          <Table containerClassName="min-h-0 flex-1 overflow-auto">
            <TableHeader className="sticky top-0 z-10 bg-background">
              <TableRow>
                <TableHead className="min-w-52">模型</TableHead>
                <TableHead className="w-40 text-right">输入价格</TableHead>
                <TableHead className="w-40 text-right">输出价格</TableHead>
                <TableHead className="w-40 text-right">缓存创建</TableHead>
                <TableHead className="w-40 text-right">缓存读取</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pagination.visibleItems.map((row) => {
                const prices = modelPriceColumnValues(row);
                const tiers = row.billing_expr ? expressionPriceTiers(row.billing_expr) : [];
                const showTiers = tiers.length > 1;
                return (
                  <TableRow key={row.model}>
                    <TableCell className="align-top font-mono text-xs font-medium">
                      <div
                        className={
                          showTiers
                            ? "grid grid-cols-[minmax(0,1fr)_minmax(7rem,10rem)] items-start gap-5"
                            : "flex min-h-5 items-baseline gap-2"
                        }
                      >
                        <div className="min-w-0">
                          <TableOverflowTooltip content={row.model}>
                            {row.model}
                          </TableOverflowTooltip>
                          <div className="text-muted-foreground mt-1 font-sans text-[11px] font-normal">
                            {showTiers ? `阶梯计费 · ${tiers.length} 档` : billingModeLabel(row)}
                          </div>
                        </div>
                        {showTiers ? <TierLabels tiers={tiers} /> : null}
                      </div>
                    </TableCell>
                    <TableCell
                      className="align-top text-right font-mono text-xs"
                      aria-label={
                        showTiers
                          ? `${row.model} 输入价格：${tierPriceLabel(tiers, "input")}`
                          : undefined
                      }
                    >
                      {showTiers ? (
                        <TierPriceValues tiers={tiers} field="input" />
                      ) : (
                        prices.input || "-"
                      )}
                    </TableCell>
                    <TableCell
                      className="align-top text-right font-mono text-xs"
                      aria-label={
                        showTiers
                          ? `${row.model} 输出价格：${tierPriceLabel(tiers, "output")}`
                          : undefined
                      }
                    >
                      {showTiers ? (
                        <TierPriceValues tiers={tiers} field="output" />
                      ) : (
                        prices.output || "-"
                      )}
                    </TableCell>
                    <TableCell
                      className="align-top text-right font-mono text-xs"
                      aria-label={`${row.model} 缓存创建价格：${
                        showTiers ? tierCacheCreatePriceLabel(tiers) : cacheCreatePriceLabel(prices)
                      }`}
                    >
                      {showTiers ? (
                        <TierCacheCreatePrices tiers={tiers} />
                      ) : (
                        <CacheCreatePrices prices={prices} />
                      )}
                    </TableCell>
                    <TableCell
                      className="align-top text-right font-mono text-xs"
                      aria-label={
                        showTiers
                          ? `${row.model} 缓存读取价格：${tierPriceLabel(tiers, "cacheRead")}`
                          : undefined
                      }
                    >
                      {showTiers ? (
                        <TierPriceValues tiers={tiers} field="cacheRead" />
                      ) : (
                        prices.cacheRead || "-"
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
          {rows.length > 0 ? (
            <DataTablePagination
              currentPage={pagination.currentPage}
              totalPages={pagination.totalPages}
              totalItems={rows.length}
              pageSize={pagination.pageSize}
              pageSizes={[10, 20, 50, 100]}
              onPageChange={pagination.setCurrentPage}
              onPageSizeChange={pagination.setPageSize}
            />
          ) : null}
        </DataTablePanel>
      )}
    </div>
  );
}
