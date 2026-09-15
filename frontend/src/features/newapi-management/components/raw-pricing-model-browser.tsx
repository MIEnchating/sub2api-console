import { useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { ChevronRight, Search } from "lucide-react";

import { DataTablePagination } from "@/components/data-table/pagination";
import { Button } from "@/components/ui/button";
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
import { rawPricingMode, rawPricingPerMillion, rawPricingText } from "../lib/raw-pricing-source";
import type { RawPricingEntry } from "../lib/raw-pricing-source";
import { RawPricingModelDetail } from "./raw-pricing-model-detail";

const priceColumns = [
  { key: "input_cost_per_token", label: "输入" },
  { key: "output_cost_per_token", label: "输出" },
  { key: "cache_read_input_token_cost", label: "缓存读取" },
  { key: "cache_creation_input_token_cost", label: "缓存写入" },
];

export function RawPricingModelBrowser(props: { entries: RawPricingEntry[] }): ReactNode {
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const lastSelected = useRef<string | null>(null);
  const list = useRef<HTMLDivElement>(null);
  const entries = useMemo(() => {
    const query = search.trim().toLowerCase();
    return props.entries.filter((entry) =>
      `${entry.model} ${rawPricingText(entry.fields.litellm_provider)}`
        .toLowerCase()
        .includes(query),
    );
  }, [props.entries, search]);
  const pagination = useClientPagination(entries);
  const selection = props.entries.find((entry) => entry.model === selected);
  useEffect(() => {
    if (selection || !lastSelected.current) return;
    const buttons = list.current?.querySelectorAll<HTMLButtonElement>("button[data-model]");
    Array.from(buttons ?? [])
      .find((button) => button.dataset.model === lastSelected.current)
      ?.focus();
  }, [selection]);

  if (selection)
    return <RawPricingModelDetail entry={selection} onBack={() => setSelected(null)} />;

  return (
    <div ref={list} className="flex h-full min-h-0 min-w-0 flex-col gap-3">
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-2">
        <div className="relative w-full sm:w-72">
          <Search
            aria-hidden="true"
            className="text-muted-foreground pointer-events-none absolute top-2 left-2 size-4"
          />
          <Input
            aria-label="搜索模型或厂商"
            placeholder="搜索模型或厂商"
            className="pl-8"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              pagination.setCurrentPage(1);
            }}
          />
        </div>
        <span className="text-muted-foreground text-xs">价格：来源币种 / 百万 Token</span>
      </div>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border">
        {entries.length === 0 ? (
          <div
            role="status"
            className="text-muted-foreground grid min-h-0 flex-1 place-items-center overflow-auto p-4 text-sm"
          >
            {props.entries.length === 0 ? "价卡中暂无模型" : "没有匹配的模型"}
          </div>
        ) : (
          <Table
            aria-label="远程价卡模型"
            className="min-w-[44rem] table-fixed"
            containerClassName="min-h-0 flex-1 overflow-auto overscroll-contain"
            overflowTooltip={false}
          >
            <TableHeader>
              <TableRow>
                <TableHead className="w-[40%]">模型</TableHead>
                {priceColumns.map((column) => (
                  <TableHead key={column.key} className="text-right">
                    {column.label}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {pagination.visibleItems.map((entry) => (
                <TableRow key={entry.model}>
                  <TableCell className="whitespace-normal">
                    <Button
                      variant="ghost"
                      className="h-auto min-h-8 w-full justify-start gap-2 px-1 text-left"
                      aria-label={`查看 ${entry.model} 明细`}
                      data-model={entry.model}
                      onClick={() => {
                        lastSelected.current = entry.model;
                        setSelected(entry.model);
                      }}
                    >
                      <span className="min-w-0 flex-1 font-mono text-xs whitespace-normal [overflow-wrap:anywhere]">
                        {entry.model}
                      </span>
                      <ChevronRight aria-hidden="true" className="text-muted-foreground shrink-0" />
                    </Button>
                    <div className="text-muted-foreground px-1 text-xs [overflow-wrap:anywhere]">
                      {rawPricingText(entry.fields.litellm_provider)} ·{" "}
                      {rawPricingMode(entry.fields.mode)}
                    </div>
                  </TableCell>
                  {priceColumns.map((column) => (
                    <TableCell key={column.key} className="text-right font-mono text-xs">
                      {rawPricingPerMillion(entry.fields[column.key])}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        {entries.length > 0 ? (
          <DataTablePagination
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            totalItems={entries.length}
            pageSize={pagination.pageSize}
            onPageChange={pagination.setCurrentPage}
            onPageSizeChange={pagination.setPageSize}
          />
        ) : null}
      </div>
    </div>
  );
}
