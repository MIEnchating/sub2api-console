import { useEffect, useRef } from "react";
import type { ReactNode } from "react";
import { ArrowLeft } from "lucide-react";

import { TableActionButton } from "@/components/data-table/table-action-button";
import { JsonEditor } from "@/components/json-editor";
import {
  rawPricingFieldLabels,
  rawPricingOtherPriceLabels,
  rawPricingTokenLabels,
} from "../constants";
import { rawPricingMode, rawPricingPerMillion, rawPricingText } from "../lib/raw-pricing-source";
import type { RawPricingEntry } from "../lib/raw-pricing-source";

type DisplayField = { key: string; label: string; value: string };

function FieldGroup(props: { title: string; fields: DisplayField[] }): ReactNode {
  if (props.fields.length === 0) return null;
  return (
    <section className="min-w-0">
      <h3 className="mb-2 text-sm font-medium">{props.title}</h3>
      <dl className="grid gap-x-6 divide-y sm:grid-cols-2">
        {props.fields.map((field) => (
          <div
            key={field.key}
            className="flex min-w-0 flex-wrap items-baseline justify-between gap-x-3 gap-y-1 py-2"
          >
            <dt className="text-muted-foreground text-xs [overflow-wrap:anywhere]">
              {field.label}
            </dt>
            <dd className="min-w-0 text-sm tabular-nums whitespace-pre-wrap [overflow-wrap:anywhere]">
              {field.value}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

export function RawPricingModelDetail(props: {
  entry: RawPricingEntry;
  onBack: () => void;
}): ReactNode {
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => heading.current?.focus(), []);
  const tokenPrices: DisplayField[] = [];
  const otherPrices: DisplayField[] = [];
  const metadata: DisplayField[] = [];
  const capabilities: DisplayField[] = [];
  const other: Array<[string, unknown]> = [];
  for (const [key, value] of Object.entries(props.entry.fields)) {
    if (Object.hasOwn(rawPricingTokenLabels, key)) {
      tokenPrices.push({
        key,
        label: rawPricingTokenLabels[key],
        value: rawPricingPerMillion(value),
      });
    } else if (Object.hasOwn(rawPricingOtherPriceLabels, key)) {
      otherPrices.push({
        key,
        label: rawPricingOtherPriceLabels[key],
        value: rawPricingText(value),
      });
    } else if (Object.hasOwn(rawPricingFieldLabels, key)) {
      const target = key.startsWith("supports_") ? capabilities : metadata;
      target.push({
        key,
        label: rawPricingFieldLabels[key],
        value: key === "mode" ? rawPricingMode(value) : rawPricingText(value),
      });
    } else {
      other.push([key, value]);
    }
  }
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
      <div className="flex min-w-0 shrink-0 items-start gap-2">
        <TableActionButton label="返回模型列表" onClick={props.onBack}>
          <ArrowLeft aria-hidden="true" />
        </TableActionButton>
        <h2
          ref={heading}
          tabIndex={-1}
          className="min-w-0 py-1 font-mono text-sm font-semibold outline-none [overflow-wrap:anywhere]"
        >
          {props.entry.model}
        </h2>
      </div>
      <div
        role="region"
        aria-label="模型详情"
        tabIndex={0}
        className="min-h-0 min-w-0 flex-1 space-y-5 overflow-auto overscroll-contain pr-1"
      >
        {Object.keys(props.entry.fields).length === 0 ? (
          <p role="status" className="text-muted-foreground text-sm">
            该模型暂无字段
          </p>
        ) : null}
        <FieldGroup title="Token 价格（来源币种 / 百万 Token）" fields={tokenPrices} />
        <FieldGroup title="其他计费单位" fields={otherPrices} />
        <FieldGroup title="模型信息" fields={metadata} />
        <FieldGroup title="模型能力" fields={capabilities} />
        {other.length > 0 ? (
          <details className="min-w-0 border-t pt-3">
            <summary className="cursor-pointer text-sm font-medium">
              其他字段（{other.length}）
            </summary>
            <JsonEditor
              className="mt-2"
              aria-label="其他字段 JSON"
              readOnly
              value={JSON.stringify(Object.fromEntries(other), null, 2)}
            />
          </details>
        ) : null}
      </div>
    </div>
  );
}
