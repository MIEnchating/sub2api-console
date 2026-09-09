import { useId, type ReactElement } from "react";
import type { AccountStatus } from "@/api";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

const latencyFormatter = new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 2 });

function latencyLabel(value: number | null): string {
  if (value === null || !Number.isFinite(value) || value <= 0) return "—";
  if (value < 1) return "<1ms";
  if (value < 1000) return `${latencyFormatter.format(value)}ms`;
  return `${latencyFormatter.format(value / 1000)}s`;
}

export function AccountLatencyCell(props: {
  account: Pick<AccountStatus, "ttfb_p50_ms" | "ttfb_p95_ms">;
}): ReactElement {
  const descriptionId = useId();
  const p95 = latencyLabel(props.account.ttfb_p95_ms);
  const p50 = latencyLabel(props.account.ttfb_p50_ms);
  const missing = p95 === "—" && p50 === "—";

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div
            className="focus-visible:ring-ring grid w-fit min-w-24 cursor-help gap-1.5 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-offset-2"
            tabIndex={0}
            aria-label="真实流量首字延迟说明"
            aria-describedby={descriptionId}
          />
        }
      >
        <dl className="grid w-fit grid-cols-[auto_auto] items-baseline gap-x-2 gap-y-1 text-xs tabular-nums">
          <dt className="text-muted-foreground">P95</dt>
          <dd className="text-sm font-semibold">{p95}</dd>
          <dt className="text-muted-foreground">P50</dt>
          <dd className="font-medium">{p50}</dd>
        </dl>
        {missing ? <span className="text-muted-foreground text-xs">暂无首字数据</span> : null}
      </TooltipTrigger>
      <TooltipContent id={descriptionId} role="tooltip" className="grid max-w-72 gap-2 text-xs">
        <strong>真实流量首字延迟</strong>
        <p>
          仅统计有效期内成功真实请求的首字耗时；探针和请求总耗时不计入。
          每个分组按有效首字样本最多的模型统计，P50 为该模型的中位数，P95 表示该模型 95%
          的样本首字耗时不超过此值。多分组账号展示各分组分位数的均值。
        </p>
        <p>
          没有符合条件的样本，或尚未完成调度评估时显示「—」。请确认流量采集已开启、真实请求已返回首字计时，并等待下一轮调度评估。
        </p>
      </TooltipContent>
    </Tooltip>
  );
}
