import { useEffect, useMemo, useState } from "react";
import { CircleDollarSign, Save, Search } from "lucide-react";

import type { NewAPIModelPrice, NewAPIRemoteSnapshot } from "@/api";
import { Badge } from "@/components/ui/badge";
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

type PriceProps = {
  models: NewAPIModelPrice[];
  pending: boolean;
  onSave: (prices: NewAPIModelPrice[]) => void;
};

export function NewAPIModelPrices(props: PriceProps) {
  const [search, setSearch] = useState("");
  const [drafts, setDrafts] = useState<Record<string, NewAPIModelPrice>>({});

  useEffect(() => {
    setDrafts(Object.fromEntries(props.models.map((model) => [model.model, { ...model }])));
  }, [props.models]);

  const visibleModels = useMemo(() => {
    const query = search.trim().toLocaleLowerCase();
    return query
      ? props.models.filter((model) => model.model.toLocaleLowerCase().includes(query))
      : props.models;
  }, [props.models, search]);

  const changed = props.models.flatMap((model) => {
    const draft = drafts[model.model];
    if (!draft) return [];
    return draft.input_ratio !== model.input_ratio ||
      draft.completion_ratio !== model.completion_ratio
      ? [draft]
      : [];
  });

  return (
    <section className="overflow-hidden rounded-md border bg-background">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold">模型价格</h2>
          <p className="text-muted-foreground mt-0.5 text-xs">{props.models.length} 个模型</p>
        </div>
        <div className="flex items-center gap-2">
          <label className="relative w-52 max-w-full">
            <Search
              className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
              aria-hidden="true"
            />
            <Input
              className="pl-8"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="搜索模型"
              aria-label="搜索模型"
            />
          </label>
          <Button
            size="sm"
            disabled={props.pending || changed.length === 0}
            onClick={() => props.onSave(changed)}
          >
            <Save aria-hidden="true" />
            {props.pending ? "正在保存" : `保存修改${changed.length ? ` (${changed.length})` : ""}`}
          </Button>
        </div>
      </div>
      {props.models.length === 0 ? (
        <div className="text-muted-foreground flex min-h-52 flex-col items-center justify-center gap-2 px-6 text-sm">
          <CircleDollarSign className="size-8 opacity-45" aria-hidden="true" />
          <span>尚未读取到模型价格</span>
        </div>
      ) : (
        <div className="max-h-[calc(100svh-18rem)] overflow-auto">
          <Table>
            <TableHeader className="sticky top-0 z-10 bg-background">
              <TableRow>
                <TableHead>模型</TableHead>
                <TableHead className="w-48">输入倍率</TableHead>
                <TableHead className="w-48">补全倍率</TableHead>
                <TableHead className="w-24">状态</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visibleModels.map((model) => {
                const draft = drafts[model.model] ?? model;
                const isChanged =
                  draft.input_ratio !== model.input_ratio ||
                  draft.completion_ratio !== model.completion_ratio;
                return (
                  <TableRow key={model.model}>
                    <TableCell className="font-mono text-xs font-medium">{model.model}</TableCell>
                    <TableCell>
                      <Input
                        className="font-mono text-xs"
                        inputMode="decimal"
                        value={draft.input_ratio}
                        aria-label={`${model.model} 输入倍率`}
                        onChange={(event) =>
                          setDrafts((current) => ({
                            ...current,
                            [model.model]: { ...draft, input_ratio: event.target.value },
                          }))
                        }
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        className="font-mono text-xs"
                        inputMode="decimal"
                        value={draft.completion_ratio}
                        aria-label={`${model.model} 补全倍率`}
                        onChange={(event) =>
                          setDrafts((current) => ({
                            ...current,
                            [model.model]: { ...draft, completion_ratio: event.target.value },
                          }))
                        }
                      />
                    </TableCell>
                    <TableCell>
                      {isChanged ? (
                        <Badge variant="outline">已修改</Badge>
                      ) : (
                        <span className="text-muted-foreground text-xs">已同步</span>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  );
}

const differenceLabels = {
  missing_in_newapi: "New API 缺失",
  only_in_newapi: "仅 New API",
  ratio_mismatch: "倍率不同",
} as const;

export function NewAPIPriceDifferences(props: { snapshot: NewAPIRemoteSnapshot }) {
  return (
    <section className="overflow-hidden rounded-md border bg-background">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold">价格差异</h2>
          <p className="text-muted-foreground mt-0.5 text-xs">
            {props.snapshot.differences.length} 项差异 · 采集于{" "}
            {new Date(props.snapshot.fetched_at).toLocaleString("zh-CN")}
          </p>
        </div>
        <Badge variant={props.snapshot.differences.length ? "destructive" : "secondary"}>
          {props.snapshot.differences.length ? "需要处理" : "未发现差异"}
        </Badge>
      </div>
      {props.snapshot.references.length === 0 ? (
        <div className="text-muted-foreground flex min-h-52 items-center justify-center px-6 text-sm">
          当前平台未返回可比较的公开价格目录
        </div>
      ) : (
        <div className="max-h-[calc(100svh-18rem)] overflow-auto">
          <Table>
            <TableHeader className="sticky top-0 z-10 bg-background">
              <TableRow>
                <TableHead>模型</TableHead>
                <TableHead>差异</TableHead>
                <TableHead>New API 输入 / 补全</TableHead>
                <TableHead>价格目录输入 / 补全</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.snapshot.differences.map((difference) => (
                <TableRow key={difference.model}>
                  <TableCell className="font-mono text-xs font-medium">
                    {difference.model}
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline">{differenceLabels[difference.kind]}</Badge>
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {difference.configured
                      ? `${difference.configured.input_ratio} / ${difference.configured.completion_ratio}`
                      : "-"}
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {difference.reference
                      ? `${difference.reference.input_ratio} / ${difference.reference.completion_ratio}`
                      : "-"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  );
}
