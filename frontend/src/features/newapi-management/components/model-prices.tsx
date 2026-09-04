import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import { CircleDollarSign, FileText, GitCompareArrows, Search } from "lucide-react";

import type {
  NewAPIModelPrice,
  NewAPIRemoteSnapshot,
  NewAPIToolPrice,
  Sub2APIModelPrice,
} from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { TableOverflowTooltip } from "@/components/ui/table-overflow-tooltip";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { formatModelPriceNumber, modelPriceNumbersEqual } from "../lib/pricing-number";
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
  toolPrices?: NewAPIToolPrice[];
  managementPrices?: Sub2APIModelPrice[];
  managementPricesPending?: boolean;
  managementPricesError?: string;
  onViewManagementPrices?: () => void;
  onCompareManagementPrices?: () => void;
  onViewRawPricingSource?: () => void;
  onWriteManagementPrice?: (price: Sub2APIModelPrice) => void;
  writingManagementPrice?: string;
  writtenManagementPrice?: NewAPIModelPrice | null;
  onWrittenManagementPriceOpenChange?: (open: boolean) => void;
};

type PriceTab = "models" | "unset" | "tools" | "remote";

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

  function showRemotePrices(model: string) {
    setRemoteSearch(model);
    setTab("remote");
    props.onViewManagementPrices?.();
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
          {props.onViewRawPricingSource ? (
            <Button size="sm" variant="outline" onClick={props.onViewRawPricingSource}>
              <FileText aria-hidden="true" />
              查看原始价卡
            </Button>
          ) : null}
          {props.onCompareManagementPrices ? (
            <Button
              size="sm"
              variant="outline"
              disabled={props.managementPricesPending}
              onClick={() => {
                setComparisonRequested(true);
                props.onCompareManagementPrices?.();
              }}
            >
              <GitCompareArrows aria-hidden="true" />
              {comparisonRequested && props.managementPricesPending ? "正在比较" : "比较模型价格"}
            </Button>
          ) : null}
        </div>
      </TableFilterToolbar>
      <SegmentedControl role="tablist" aria-label="价格分类">
        <SegmentedControlItem
          id="price-tab-models"
          role="tab"
          aria-controls="price-panel-models"
          selected={tab === "models"}
          onClick={() => setTab("models")}
        >
          模型价格
        </SegmentedControlItem>
        <SegmentedControlItem
          id="price-tab-unset"
          role="tab"
          aria-controls="price-panel-unset"
          selected={tab === "unset"}
          onClick={() => setTab("unset")}
        >
          未设置模型价格
        </SegmentedControlItem>
        <SegmentedControlItem
          id="price-tab-tools"
          role="tab"
          aria-controls="price-panel-tools"
          selected={tab === "tools"}
          onClick={() => setTab("tools")}
        >
          工具价格
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
          />
        ) : null}
        {tab === "tools" ? <ToolPricesTable prices={props.toolPrices ?? []} /> : null}
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
                  <TableHead className="min-w-52">模型</TableHead>
                  <TableHead className="w-40 text-right">输入价格</TableHead>
                  <TableHead className="w-40 text-right">输出价格</TableHead>
                  <TableHead className="w-40 text-right">缓存创建</TableHead>
                  <TableHead className="w-40 text-right">缓存读取</TableHead>
                  {tab === "models" ? <TableHead className="w-28">状态</TableHead> : null}
                  <TableHead className="w-28 text-right">操作</TableHead>
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
                          />
                        </TableCell>
                      ) : null}
                      <TableCell className="align-top text-right">
                        {tab === "unset" ? (
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            onClick={() => showRemotePrices(row.model)}
                          >
                            <Search aria-hidden="true" />
                            查询
                          </Button>
                        ) : null}
                        {tab === "models" && comparisonStatus === "mismatched" ? (
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              const remote = props.managementPrices?.find(
                                (price) => price.model === row.model,
                              );
                              if (remote) {
                                setDifferenceSelection({ configured: row.configured, remote });
                              }
                            }}
                          >
                            查看差异
                          </Button>
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
      <ModelPriceDifferenceDialog
        selection={differenceSelection}
        onOpenChange={(open) => {
          if (!open) setDifferenceSelection(null);
        }}
      />
      <WrittenModelPriceDialog
        price={props.writtenManagementPrice ?? null}
        onOpenChange={(open) => props.onWrittenManagementPriceOpenChange?.(open)}
      />
    </div>
  );
}

export function RemoteModelPricesTable(props: {
  prices: Sub2APIModelPrice[];
  pending: boolean;
  error: string;
  filtered?: boolean;
  writingModel?: string;
  onWritePrice?: (price: Sub2APIModelPrice) => void;
}) {
  const pagination = useClientPagination(props.prices, 10);

  return (
    <DataTablePanel className="flex min-h-0 flex-1 flex-col">
      {props.pending ? (
        <div
          className="text-muted-foreground grid min-h-52 place-items-center text-sm"
          role="status"
        >
          正在获取远程模型价格
        </div>
      ) : props.error ? (
        <div className="text-destructive grid min-h-52 place-items-center px-6 text-center text-sm">
          {props.error}
        </div>
      ) : props.prices.length === 0 ? (
        <div className="text-muted-foreground grid min-h-52 place-items-center px-6 text-center text-sm">
          {props.filtered ? "没有匹配的模型" : "远程价卡未返回模型价格"}
        </div>
      ) : (
        <Table
          containerClassName="min-h-0 flex-1 overflow-auto"
          overflowTooltip={false}
          className="min-w-[64rem]"
        >
          <TableHeader>
            <TableRow>
              <TableHead className="min-w-56">模型</TableHead>
              <TableHead className="text-right">输入价格（$/百万 Token）</TableHead>
              <TableHead className="text-right">输出价格（$/百万 Token）</TableHead>
              <TableHead className="text-right">缓存写入</TableHead>
              <TableHead className="text-right">缓存读取</TableHead>
              <TableHead className="text-right">图片价格（USD）</TableHead>
              {props.onWritePrice ? <TableHead className="w-36 text-right">操作</TableHead> : null}
            </TableRow>
          </TableHeader>
          <TableBody>
            {pagination.visibleItems.map((price) => {
              const writeSupported = remotePriceSupportsNewAPIWrite(price);
              return (
                <TableRow key={price.model}>
                  <TableCell className="font-mono text-xs font-medium">
                    <div>{price.model}</div>
                    {price.long_context_threshold ? (
                      <div className="text-muted-foreground mt-1 font-sans text-[11px] font-normal">
                        阶梯 {formatRemoteThreshold(price.long_context_threshold)}
                      </div>
                    ) : null}
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    {managementTierPrice(price.input_price, price.long_context_input_price)}
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    {managementTierPrice(price.output_price, price.long_context_output_price)}
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    <ManagementCacheWritePrice price={price} />
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    {managementTierPrice(
                      price.cache_read_price,
                      price.long_context_cache_read_price,
                    )}
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    <ManagementImagePrice price={price} />
                  </TableCell>
                  {props.onWritePrice ? (
                    <TableCell className="text-right">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={props.writingModel !== undefined || !writeSupported}
                        onClick={() => props.onWritePrice?.(price)}
                      >
                        {remoteWriteButtonLabel(writeSupported, props.writingModel === price.model)}
                      </Button>
                    </TableCell>
                  ) : null}
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}
      {!props.pending && !props.error && props.prices.length > 0 ? (
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

function remoteWriteButtonLabel(writeSupported: boolean, writing: boolean): string {
  if (!writeSupported) return "暂不支持写入";
  if (writing) return "正在写入";
  return "写入 New API";
}

function remotePriceBillingExpression(price: Sub2APIModelPrice): string {
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

function pricePerMillion(value?: string): string {
  const normalized = value?.trim() ?? "";
  const match = /^(\+?)(\d+)(?:\.(\d*))?(?:e([+-]?\d+))?$/i.exec(normalized);
  if (!match) return "";
  const whole = match[2];
  const fraction = match[3] ?? "";
  const exponent = Number(match[4] ?? "0");
  if (!Number.isSafeInteger(exponent) || Math.abs(exponent) > 100) return "";
  const digits = `${whole}${fraction}`;
  if (!/[1-9]/.test(digits)) return "0";
  const decimalPosition = whole.length + exponent + 6;
  let result: string;
  if (decimalPosition <= 0) {
    result = `0.${"0".repeat(-decimalPosition)}${digits}`;
  } else if (decimalPosition >= digits.length) {
    result = `${digits}${"0".repeat(decimalPosition - digits.length)}`;
  } else {
    result = `${digits.slice(0, decimalPosition)}.${digits.slice(decimalPosition)}`;
  }
  const parts = result.split(".");
  const normalizedWhole = parts[0].replace(/^0+(?=\d)/, "");
  const normalizedFraction = (parts[1] ?? "").replace(/0+$/, "");
  const shifted = normalizedFraction ? `${normalizedWhole}.${normalizedFraction}` : normalizedWhole;
  return formatModelPriceNumber(shifted);
}

export type NewAPIPriceComparisonStatus = "matched" | "mismatched" | "missing";

export function newAPIPriceComparisonStatus(
  configured: NewAPIModelPrice,
  remotePrices: Sub2APIModelPrice[],
): NewAPIPriceComparisonStatus {
  const remote = remotePrices.find((price) => price.model === configured.model);
  if (!remote) return "missing";
  const expected = remotePriceToNewAPIModelPrice(remote);
  if (expected.billing_mode === "tiered_expr") {
    if (configured.billing_mode !== "tiered_expr" || !configured.billing_expr?.trim()) {
      return "mismatched";
    }
    if (!tieredExpressionStructureMatches(configured.billing_expr, expected.billing_expr ?? "")) {
      return "mismatched";
    }
    const configuredPrices = modelPriceColumnValues(configured);
    const expectedPrices = modelPriceColumnValues(expected);
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
  const expectedPrices = modelPriceColumnValues(expected);
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

function ModelPriceComparisonStatus(props: {
  configured: NewAPIModelPrice;
  remotePrices?: Sub2APIModelPrice[];
  requested: boolean;
  pending: boolean;
  error: string;
}) {
  if (!props.requested) return <StatusBadge label="未比较" variant="neutral" />;
  if (props.error) return <StatusBadge label="比较失败" variant="danger" />;
  if (props.pending || props.remotePrices === undefined) {
    return <StatusBadge label="比较中" variant="info" pulse />;
  }

  const status = newAPIPriceComparisonStatus(props.configured, props.remotePrices);
  if (status === "matched") return <StatusBadge label="一致" variant="success" />;
  if (status === "missing") return <StatusBadge label="远程未找到" variant="neutral" />;
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
  const remotePrices = modelPriceColumnValues(expected);
  const configuredMode = billingModeLabel(configured);
  const remoteMode = billingModeLabel(expected);
  const rows: Array<{
    label: string;
    configured: string;
    remote: string;
    kind: "text" | "decimal";
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
      configured: configuredPrices.cacheCreate,
      remote: remotePrices.cacheCreate,
      kind: "decimal",
    },
    {
      label: "缓存写入（1h）（$/百万 Token）",
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
      configured: configuredPrices.imageInput,
      remote: remotePrices.imageInput,
      kind: "decimal",
    },
  ];
  const configuredCondition = tierConditionSignature(configured.billing_expr ?? "");
  const remoteCondition = tierConditionSignature(expected.billing_expr ?? "");
  if (configuredCondition || remoteCondition) {
    rows.splice(1, 0, {
      label: "阶梯条件",
      configured: configuredCondition,
      remote: remoteCondition,
      kind: "text",
    });
  }
  return rows.map((row) => ({
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
          <DialogDescription>当前 New API 配置与远程价卡的价格对照。</DialogDescription>
        </DialogHeader>
        <DialogBody>
          <Table overflowTooltip={false}>
            <TableHeader>
              <TableRow>
                <TableHead>价格项</TableHead>
                <TableHead className="text-right">当前 New API</TableHead>
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
          <DialogDescription>New API 写入后重新读取到的实际配置。</DialogDescription>
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
            <TableHead className="text-right">New API 读回结果</TableHead>
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

function ToolPricesTable(props: { prices: NewAPIToolPrice[] }) {
  if (props.prices.length === 0) {
    return (
      <div className="text-muted-foreground flex min-h-52 items-center justify-center px-6 text-sm">
        尚未配置工具价格
      </div>
    );
  }
  return (
    <DataTablePanel className="flex-1">
      <Table containerClassName="min-h-0 flex-1 overflow-auto">
        <TableHeader>
          <TableRow>
            <TableHead>工具</TableHead>
            <TableHead>价格（$/1K 次）</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.prices.map((price) => (
            <TableRow key={price.tool}>
              <TableCell className="font-mono text-xs font-medium">{price.tool}</TableCell>
              <TableCell className="font-mono text-xs">{price.price}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </DataTablePanel>
  );
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
