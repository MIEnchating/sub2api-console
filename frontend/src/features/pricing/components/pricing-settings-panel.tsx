import type { ReactElement } from "react";
import { CircleDollarSign } from "lucide-react";

import { FieldLabel } from "@/components/field-help-tooltip";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import type { PricingConfigDraft } from "../types";

export function PricingSettingsPanel(props: {
  value: PricingConfigDraft;
  onChange: (value: PricingConfigDraft) => void;
}): ReactElement {
  return (
    <Card
      size="sm"
      role="region"
      aria-labelledby="pricing-settings-title"
      className="rounded-xl xl:sticky xl:top-1"
      data-testid="pricing-settings-panel"
    >
      <CardHeader className="bg-muted/20 grid-cols-1 gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span className="bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md">
            <CircleDollarSign className="size-4" aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <CardTitle id="pricing-settings-title">自动价格分组</CardTitle>
            <CardDescription className="mt-1 text-xs leading-5">
              设置盈利目标和自动执行参数。
            </CardDescription>
          </div>
        </div>
        <div className="bg-background flex w-full items-center justify-between gap-3 rounded-lg border px-3 py-2.5">
          <Badge variant={props.value.enabled ? "default" : "secondary"}>
            {props.value.enabled ? "已开启" : "默认关闭"}
          </Badge>
          <Switch
            checked={props.value.enabled}
            onCheckedChange={(enabled) => props.onChange({ ...props.value, enabled })}
            aria-label="启用动态价格分组"
          />
        </div>
      </CardHeader>
      <CardContent
        className="grid divide-y p-0 lg:grid-cols-3 lg:divide-x lg:divide-y-0 xl:grid-cols-1 xl:divide-x-0 xl:divide-y"
        data-testid="pricing-settings-grid"
      >
        <div
          className="grid min-w-0 grid-cols-[minmax(0,1fr)_8rem] items-center gap-2.5 px-3 py-3 sm:grid-cols-[minmax(0,1fr)_9rem] lg:grid-cols-1 lg:items-start lg:py-4"
          data-testid="pricing-goal-settings"
        >
          <FieldLabel
            label="目标盈利比例"
            description="利润 ÷ 账号成本；允许范围 0% - 99%"
            htmlFor="pricing-profit-margin"
          />
          <span className="relative block">
            <Input
              id="pricing-profit-margin"
              className="pr-8 tabular-nums"
              type="number"
              min={0}
              max={99}
              step="0.1"
              value={
                props.value.profit_margin === null
                  ? ""
                  : Number((props.value.profit_margin * 100).toFixed(4))
              }
              onChange={(event) =>
                props.onChange({
                  ...props.value,
                  profit_margin:
                    event.target.value === "" ? null : Number(event.target.value) / 100,
                })
              }
            />
            <span className="text-muted-foreground pointer-events-none absolute inset-y-0 right-2.5 flex items-center text-xs">
              %
            </span>
          </span>
        </div>
        <div
          className="grid min-w-0 grid-cols-[minmax(0,1fr)_8rem] items-center gap-2.5 px-3 py-3 sm:grid-cols-[minmax(0,1fr)_9rem] lg:grid-cols-1 lg:items-start lg:py-4"
          data-testid="pricing-execution-settings"
        >
          <FieldLabel
            label="动态调整间隔"
            description="30 秒 - 24 小时"
            htmlFor="pricing-interval-seconds"
          />
          <span className="relative block">
            <Input
              id="pricing-interval-seconds"
              className="pr-9 tabular-nums"
              type="number"
              min={30}
              max={86400}
              value={props.value.interval_seconds ?? ""}
              onChange={(event) =>
                props.onChange({
                  ...props.value,
                  interval_seconds: event.target.value === "" ? null : Number(event.target.value),
                })
              }
            />
            <span className="text-muted-foreground pointer-events-none absolute inset-y-0 right-2.5 flex items-center text-xs">
              秒
            </span>
          </span>
        </div>
        <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_8rem] items-center gap-2.5 px-3 py-3 sm:grid-cols-[minmax(0,1fr)_9rem] lg:grid-cols-1 lg:items-start lg:py-4">
          <FieldLabel
            label="写入并发"
            description="允许范围 1 - 16"
            htmlFor="pricing-write-concurrency"
          />
          <Input
            id="pricing-write-concurrency"
            className="tabular-nums"
            type="number"
            min={1}
            max={16}
            value={props.value.write_concurrency ?? ""}
            onChange={(event) =>
              props.onChange({
                ...props.value,
                write_concurrency: event.target.value === "" ? null : Number(event.target.value),
              })
            }
          />
        </div>
      </CardContent>
      <div className="border-t px-3 py-3 text-xs leading-5 text-muted-foreground">
        <p>执行间隔与写入并发不参与售价计算。</p>
        <details className="mt-2">
          <summary className="focus-visible:ring-ring w-fit cursor-pointer rounded-sm font-medium outline-none focus-visible:ring-2">
            查看分组选择规则
          </summary>
          <p className="mt-2">
            每个互换组优先选择达到目标盈利比例且售价最低的分组；均未达标时选择售价最高且能覆盖成本的分组，均亏损时保留当前分组。
          </p>
        </details>
      </div>
    </Card>
  );
}
