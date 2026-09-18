import { useEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, GitCompareArrows, Search } from "lucide-react";

import type { NewAPIModelPrice, NewAPIRemoteSnapshot } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { upstreamTypeLabel } from "@/lib/domain-dictionaries";
import { modelPriceNumbersEqual } from "../lib/pricing-number";
import { dismissOperationError, notifyOperationError } from "@/lib/operation-feedback";
import { modelPriceColumnValues } from "./model-prices";

export type PlatformPriceComparisonStatus = "matched" | "mismatched" | "missing";

type UpstreamPriceCatalog = NonNullable<NewAPIRemoteSnapshot["upstream_prices"]>[number];
type PriceColumns = ReturnType<typeof modelPriceColumnValues>;
type PriceColumn = keyof PriceColumns;

const dismissedPriceWarnings = new Set<string>();

function PriceWarningToast(props: { warning?: string; fetchedAt?: string }): null {
  const key = props.warning ? `${props.fetchedAt ?? ""}:${props.warning}` : "";
  const cleanupTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const cleanupTimerKey = useRef("");
  useEffect(() => {
    if (cleanupTimer.current && cleanupTimerKey.current === key) {
      clearTimeout(cleanupTimer.current);
      cleanupTimer.current = undefined;
      cleanupTimerKey.current = "";
    }
    if (!props.warning || !key || dismissedPriceWarnings.has(key)) return;
    notifyOperationError(props.warning, "上游价格读取失败");
    return () => {
      cleanupTimer.current = setTimeout(() => {
        dismissedPriceWarnings.add(key);
        dismissOperationError(props.warning, "上游价格读取失败");
      }, 0);
      cleanupTimerKey.current = key;
    };
  }, [key, props.warning]);
  return null;
}

function upstreamDisplayLabel(upstream: UpstreamPriceCatalog): string {
  return `${upstream.name.trim() || upstream.host} · ${upstreamTypeLabel(upstream.upstream_type)}`;
}

const comparedPriceColumns: Array<{ key: PriceColumn; label: string }> = [
  { key: "input", label: "输入价格" },
  { key: "output", label: "输出价格" },
  { key: "cacheCreate", label: "缓存创建" },
  { key: "cacheCreate1h", label: "缓存创建（1 小时）" },
  { key: "cacheRead", label: "缓存读取" },
  { key: "imageInput", label: "图片输入" },
  { key: "imageOutput", label: "图片输出" },
  { key: "audioInput", label: "音频输入" },
  { key: "audioOutput", label: "音频输出" },
];

function normalizedBillingMode(price: NewAPIModelPrice): string {
  const mode = price.billing_mode?.trim();
  if (mode === "per_second") return "per-second";
  if (mode) return mode;
  if (price.billing_expr?.trim()) return "tiered_expr";
  if (price.model_price?.trim()) return "per-request";
  return "per-token";
}

function normalizedBillingExpression(price: NewAPIModelPrice): string {
  if (normalizedBillingMode(price) !== "tiered_expr") return "";
  return price.billing_expr?.replace(/\s+/g, "") ?? "";
}

function billingModeLabel(price: NewAPIModelPrice): string {
  const mode = normalizedBillingMode(price);
  if (mode === "per-request") return "按次";
  if (mode === "per-second") return "按秒";
  if (mode === "tiered_expr") return "阶梯计费";
  return "按 Token";
}

function findUpstreamModel(
  model: string,
  upstreamModels: NewAPIModelPrice[],
): NewAPIModelPrice | undefined {
  return upstreamModels.find((candidate) => candidate.model === model);
}

export function comparePlatformModelPrice(
  platformModel: NewAPIModelPrice,
  upstreamModels: NewAPIModelPrice[],
): PlatformPriceComparisonStatus {
  const upstreamModel = findUpstreamModel(platformModel.model, upstreamModels);
  if (!upstreamModel) return "missing";
  if (normalizedBillingMode(platformModel) !== normalizedBillingMode(upstreamModel)) {
    return "mismatched";
  }
  if (normalizedBillingExpression(platformModel) !== normalizedBillingExpression(upstreamModel)) {
    return "mismatched";
  }

  const platformPrices = modelPriceColumnValues(platformModel);
  const upstreamPrices = modelPriceColumnValues(upstreamModel);
  const matched = comparedPriceColumns.every((column) =>
    modelPriceNumbersEqual(platformPrices[column.key], upstreamPrices[column.key]),
  );
  return matched ? "matched" : "mismatched";
}

function statusPresentation(status: PlatformPriceComparisonStatus | undefined): {
  label: string;
  variant: "success" | "warning" | "neutral";
} {
  if (status === "matched") return { label: "一致", variant: "success" };
  if (status === "mismatched") return { label: "不一致", variant: "warning" };
  if (status === "missing") return { label: "上游未找到", variant: "neutral" };
  return { label: "未比对", variant: "neutral" };
}

function comparisonSummary(status: PlatformPriceComparisonStatus): string {
  if (status === "matched") return "价格一致";
  if (status === "missing") return "上游未找到该模型";
  return "价格不一致";
}

function modelPriceRows(platformModel: NewAPIModelPrice, upstreamModel: NewAPIModelPrice) {
  const platformPrices = modelPriceColumnValues(platformModel);
  const upstreamPrices = modelPriceColumnValues(upstreamModel);
  const rows = comparedPriceColumns.map((column) => ({
    label: column.label,
    platform: platformPrices[column.key] || "-",
    upstream: upstreamPrices[column.key] || "-",
    matched: modelPriceNumbersEqual(platformPrices[column.key], upstreamPrices[column.key]),
  }));
  rows.unshift({
    label: "计费方式",
    platform: billingModeLabel(platformModel),
    upstream: billingModeLabel(upstreamModel),
    matched: normalizedBillingMode(platformModel) === normalizedBillingMode(upstreamModel),
  });
  const platformExpression = normalizedBillingExpression(platformModel);
  const upstreamExpression = normalizedBillingExpression(upstreamModel);
  if (platformExpression || upstreamExpression) {
    rows.splice(1, 0, {
      label: "阶梯计费表达式",
      platform: platformModel.billing_expr?.trim() || "-",
      upstream: upstreamModel.billing_expr?.trim() || "-",
      matched: platformExpression === upstreamExpression,
    });
  }
  return rows;
}

function ComparisonStatus(props: {
  model: string;
  status: PlatformPriceComparisonStatus | undefined;
}) {
  const presentation = statusPresentation(props.status);
  return (
    <StatusBadge
      aria-label={`${props.model} 比对结果`}
      label={presentation.label}
      variant={presentation.variant}
    />
  );
}

function ComparisonOverview(props: {
  total: number;
  selected: number;
  compared: number;
  matched: number;
  mismatched: number;
}) {
  return (
    <section
      role="region"
      aria-label="价格比对概览"
      className="grid shrink-0 grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-5"
    >
      {[
        ["模型总数", props.total, "text-foreground"],
        ["已选择", props.selected, "text-primary"],
        ["已比对", props.compared, "text-foreground"],
        ["一致", props.matched, "text-emerald-600 dark:text-emerald-400"],
        ["不一致", props.mismatched, "text-amber-600 dark:text-amber-400"],
      ].map(([label, value, color]) => (
        <div key={label} className="rounded-lg border bg-card px-3 py-2.5 shadow-xs">
          <p className="text-muted-foreground text-xs">{label}</p>
          <p className={`mt-1 text-lg leading-none font-semibold tabular-nums ${color}`}>{value}</p>
        </div>
      ))}
    </section>
  );
}

function UpstreamSelector(props: {
  upstreams: UpstreamPriceCatalog[];
  value: string;
  onChange: (host: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const selected = props.upstreams.find((upstream) => upstream.host === props.value);

  return (
    <div
      className="relative w-full sm:w-72"
      onBlur={(event) => {
        const nextTarget = event.relatedTarget;
        if (nextTarget instanceof Node && event.currentTarget.contains(nextTarget)) return;
        setOpen(false);
      }}
    >
      <Button
        className="w-full justify-between font-normal"
        variant="outline"
        role="combobox"
        aria-label="比对上游"
        aria-expanded={open}
        aria-controls="newapi-price-upstream-options"
        onClick={() => setOpen((current) => !current)}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown") {
            event.preventDefault();
            setOpen(true);
          }
          if (event.key === "Escape") setOpen(false);
        }}
      >
        <span className={selected ? "truncate" : "text-muted-foreground truncate"}>
          {selected ? upstreamDisplayLabel(selected) : "选择比对上游"}
        </span>
        <ChevronDown className="text-muted-foreground" aria-hidden="true" />
      </Button>
      {open ? (
        <div
          id="newapi-price-upstream-options"
          role="listbox"
          aria-label="可比对上游"
          className="bg-popover text-popover-foreground ring-foreground/10 absolute top-full left-0 z-50 mt-1 max-h-72 w-full overflow-y-auto rounded-lg p-1 shadow-md ring-1"
        >
          {props.upstreams.length === 0 ? (
            <div className="text-muted-foreground px-2 py-6 text-center text-sm">
              没有可比对的上游
            </div>
          ) : null}
          {props.upstreams.map((upstream) => {
            const selectedOption = upstream.host === props.value;
            return (
              <button
                key={upstream.host}
                type="button"
                role="option"
                aria-selected={selectedOption}
                className="hover:bg-accent focus:bg-accent flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm outline-none"
                onClick={() => {
                  props.onChange(upstream.host);
                  setOpen(false);
                }}
                onKeyDown={(event) => {
                  if (event.key !== "Escape") return;
                  setOpen(false);
                }}
              >
                <span className="min-w-0 flex-1 truncate">{upstreamDisplayLabel(upstream)}</span>
                {selectedOption ? <Check className="size-4 shrink-0" aria-hidden="true" /> : null}
              </button>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}

function PriceComparisonDialog(props: {
  platformModel: NewAPIModelPrice | null;
  upstream: UpstreamPriceCatalog | null;
  onOpenChange: (open: boolean) => void;
}) {
  const upstreamModel = props.platformModel
    ? findUpstreamModel(props.platformModel.model, props.upstream?.models ?? [])
    : undefined;
  const status = props.platformModel
    ? comparePlatformModelPrice(props.platformModel, props.upstream?.models ?? [])
    : "missing";
  const summary = statusPresentation(status);
  const rows =
    props.platformModel && upstreamModel ? modelPriceRows(props.platformModel, upstreamModel) : [];

  return (
    <Dialog open={props.platformModel !== null} onOpenChange={props.onOpenChange}>
      <DialogContent width="wide" height="adaptive">
        <DialogHeader>
          <DialogTitle>
            {props.platformModel ? `${props.platformModel.model} 价格比对` : "价格比对"}
          </DialogTitle>
          <DialogDescription>
            当前平台价格与 {props.upstream?.name ?? "所选上游"} 价卡的逐项结果。
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          <StatusBadge label={comparisonSummary(status)} variant={summary.variant} />
          {rows.length > 0 ? (
            <Table overflowTooltip={false}>
              <TableHeader>
                <TableRow>
                  <TableHead>价格项</TableHead>
                  <TableHead className="text-right">当前平台</TableHead>
                  <TableHead className="text-right">所选上游</TableHead>
                  <TableHead className="w-24 text-right">结果</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((row) => (
                  <TableRow key={row.label}>
                    <TableCell>{row.label}</TableCell>
                    <TableCell className="text-right font-mono text-xs">{row.platform}</TableCell>
                    <TableCell className="text-right font-mono text-xs">{row.upstream}</TableCell>
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
          ) : null}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

export function NewAPIPriceComparison(props: { snapshot: NewAPIRemoteSnapshot }) {
  const upstreams = props.snapshot.upstream_prices ?? [];
  const [selectedUpstreamHost, setSelectedUpstreamHost] = useState("");
  const [selectedModels, setSelectedModels] = useState<string[]>([]);
  const [results, setResults] = useState<Map<string, PlatformPriceComparisonStatus>>(new Map());
  const [search, setSearch] = useState("");
  const [dialogModel, setDialogModel] = useState<NewAPIModelPrice | null>(null);
  const selectedUpstream =
    upstreams.find((upstream) => upstream.host === selectedUpstreamHost) ?? null;
  const filteredModels = useMemo(() => {
    const query = search.trim().toLocaleLowerCase();
    return [...props.snapshot.models]
      .filter((model) => !query || model.model.toLocaleLowerCase().includes(query))
      .sort((left, right) => left.model.localeCompare(right.model));
  }, [props.snapshot.models, search]);
  const pagination = useClientPagination(filteredModels);
  const visibleModelNames = pagination.visibleItems.map((model) => model.model);
  const allVisibleSelected =
    visibleModelNames.length > 0 &&
    visibleModelNames.every((model) => selectedModels.includes(model));
  const comparisonCounts = useMemo(() => {
    let matched = 0;
    let mismatched = 0;
    for (const status of results.values()) {
      if (status === "matched") matched += 1;
      if (status === "mismatched") mismatched += 1;
    }
    return { matched, mismatched };
  }, [results]);

  function selectUpstream(host: string) {
    setSelectedUpstreamHost(host);
    setResults(new Map());
    setDialogModel(null);
  }

  function toggleModel(model: string, checked: boolean) {
    setSelectedModels((current) => {
      if (checked && !current.includes(model)) return [...current, model];
      if (!checked) return current.filter((item) => item !== model);
      return current;
    });
  }

  function toggleVisibleModels(checked: boolean) {
    setSelectedModels((current) => {
      if (!checked) return current.filter((model) => !visibleModelNames.includes(model));
      return Array.from(new Set([...current, ...visibleModelNames]));
    });
  }

  function compareSelectedModels() {
    if (!selectedUpstream) return;
    setResults((current) => {
      const next = new Map(current);
      const platformModels = new Map(props.snapshot.models.map((model) => [model.model, model]));
      const upstreamModels = new Map(selectedUpstream.models.map((model) => [model.model, model]));
      for (const modelName of selectedModels) {
        const model = platformModels.get(modelName);
        const upstreamModel = upstreamModels.get(modelName);
        if (model)
          next.set(
            modelName,
            comparePlatformModelPrice(model, upstreamModel ? [upstreamModel] : []),
          );
      }
      return next;
    });
  }

  function compareSingleModel(model: NewAPIModelPrice) {
    if (!selectedUpstream) return;
    const status = comparePlatformModelPrice(model, selectedUpstream.models);
    setResults((current) => new Map(current).set(model.model, status));
    setDialogModel(model);
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto overscroll-contain">
      <PriceWarningToast
        warning={props.snapshot.upstream_price_warning}
        fetchedAt={props.snapshot.fetched_at}
      />
      <ComparisonOverview
        total={props.snapshot.models.length}
        selected={selectedModels.length}
        compared={results.size}
        matched={comparisonCounts.matched}
        mismatched={comparisonCounts.mismatched}
      />
      <TableFilterToolbar
        aria-label="价格比对筛选与操作"
        className="rounded-lg border bg-card/60 p-2 shadow-xs sm:p-2.5"
      >
        <UpstreamSelector
          upstreams={upstreams}
          value={selectedUpstreamHost}
          onChange={selectUpstream}
        />
        <div className="relative w-full min-w-0 flex-1 sm:w-auto sm:min-w-56 sm:max-w-96">
          <Search
            className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
            aria-hidden="true"
          />
          <Input
            value={search}
            className="pl-8"
            placeholder="搜索模型"
            aria-label="搜索比对模型"
            onChange={(event) => setSearch(event.target.value)}
          />
        </div>
        <Button
          className="ml-auto w-full sm:w-auto"
          disabled={!selectedUpstream || selectedModels.length === 0}
          onClick={compareSelectedModels}
        >
          <GitCompareArrows aria-hidden="true" />
          批量比对
        </Button>
      </TableFilterToolbar>

      <DataTablePanel className="min-h-64 flex-1">
        {props.snapshot.models.length === 0 ? (
          <div className="text-muted-foreground flex min-h-52 items-center justify-center px-6 text-sm">
            尚未读取到本平台模型价格
          </div>
        ) : (
          <>
            {filteredModels.length === 0 ? (
              <div className="text-muted-foreground flex min-h-52 flex-1 items-center justify-center px-6 text-sm">
                没有匹配的模型，请调整搜索条件
              </div>
            ) : (
              <Table
                actionColumn
                containerClassName="min-h-0 flex-1 overflow-auto"
                className="min-w-[48rem]"
              >
                <TableHeader className="sticky top-0 z-10 bg-background shadow-[0_1px_0_var(--border)]">
                  <TableRow>
                    <TableHead className="w-12">
                      <Checkbox
                        checked={allVisibleSelected}
                        aria-label="选择当前页全部模型"
                        onCheckedChange={toggleVisibleModels}
                      />
                    </TableHead>
                    <TableHead>模型</TableHead>
                    <TableHead className="w-40 text-right">输入价格</TableHead>
                    <TableHead className="w-40 text-right">输出价格</TableHead>
                    <TableHead className="w-32">结果</TableHead>
                    <TableHead className="w-24 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {pagination.visibleItems.map((model) => {
                    const prices = modelPriceColumnValues(model);
                    return (
                      <TableRow key={model.model} className="transition-colors hover:bg-muted/40">
                        <TableCell>
                          <Checkbox
                            checked={selectedModels.includes(model.model)}
                            aria-label={`选择 ${model.model}`}
                            onCheckedChange={(checked) => toggleModel(model.model, checked)}
                          />
                        </TableCell>
                        <TableCell className="font-mono text-xs font-medium">
                          {model.model}
                        </TableCell>
                        <TableCell className="text-right font-mono text-xs">
                          {prices.input || "-"}
                        </TableCell>
                        <TableCell className="text-right font-mono text-xs">
                          {prices.output || "-"}
                        </TableCell>
                        <TableCell>
                          <ComparisonStatus model={model.model} status={results.get(model.model)} />
                        </TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="outline"
                            aria-label={`比对 ${model.model}`}
                            disabled={!selectedUpstream}
                            onClick={() => compareSingleModel(model)}
                          >
                            <GitCompareArrows aria-hidden="true" />
                            比对
                          </Button>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            )}
            <DataTablePagination
              currentPage={pagination.currentPage}
              totalPages={pagination.totalPages}
              totalItems={filteredModels.length}
              pageSize={pagination.pageSize}
              onPageChange={pagination.setCurrentPage}
              onPageSizeChange={pagination.setPageSize}
            />
          </>
        )}
      </DataTablePanel>

      {upstreams.length === 0 ? (
        <p className="text-muted-foreground text-xs">尚未读取到可比对的上游价卡</p>
      ) : null}
      <PriceComparisonDialog
        platformModel={dialogModel}
        upstream={selectedUpstream}
        onOpenChange={(open) => {
          if (!open) setDialogModel(null);
        }}
      />
    </div>
  );
}
