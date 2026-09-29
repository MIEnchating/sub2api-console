import type { ReactElement } from "react";
import type { AnimationResult } from "@/api";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

const tokenFormat = new Intl.NumberFormat("zh-CN");

export function formatTokens(value: number | undefined): string {
  return value === undefined ? "未返回" : tokenFormat.format(value);
}

export function ResultTokenMetrics(props: { result?: AnimationResult }): ReactElement {
  const usage = props.result?.usage;
  const duration = props.result?.generation_duration_ms;
  const output = usage?.output_tokens;
  let total = usage?.total_tokens;
  if (total === undefined && usage?.input_tokens !== undefined && output !== undefined)
    total = usage.input_tokens + output;
  let tps = "—";
  let reason = "检测完成后按输出 Token ÷ 本次生成耗时计算，不含此前失败重试与退避";
  if (props.result) {
    if (output === undefined) {
      tps = "无法计算";
      reason = "缺少输出 Token，无法计算 TPS";
    } else if (duration === undefined || duration <= 0) {
      tps = "无法计算";
      reason = "缺少有效生成耗时，无法计算 TPS";
    } else {
      tps = `${((output * 1000) / duration).toFixed(1)} TPS`;
      reason = "TPS = 输出 Token ÷ 本次生成耗时（不含此前失败重试与退避）";
    }
  }
  return (
    <dl className="grid w-full min-w-0 grid-cols-4 gap-x-2 text-xs tabular-nums">
      <Metric label="输入 Token" value={props.result ? formatTokens(usage?.input_tokens) : "—"} />
      <Metric label="输出 Token" value={props.result ? formatTokens(output) : "—"} />
      <Metric label="总计 Token" value={props.result ? formatTokens(total) : "—"} />
      <Metric label="TPS（计算）" value={tps} title={reason} />
    </dl>
  );
}

export function Metric(props: { label: string; value: string; title?: string }): ReactElement {
  const content = (
    <div className="min-w-0 space-y-1" aria-label={props.title}>
      <dt className="text-muted-foreground">{props.label}</dt>
      <dd className="font-medium wrap-anywhere">{props.value}</dd>
    </div>
  );
  if (!props.title) return content;
  return (
    <Tooltip>
      <TooltipTrigger render={content} />
      <TooltipContent>{props.title}</TooltipContent>
    </Tooltip>
  );
}
