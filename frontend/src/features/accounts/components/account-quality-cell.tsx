import type { AccountQualityStatistics, AccountQualityWindow, AccountStatus } from "@/api";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { accountCheckStatus } from "@/features/model-check/lib/account-link";

function score(window?: AccountQualityWindow): string {
  return window?.score == null ? "暂无样本" : `${window.score.toFixed(1)}%`;
}

function QualityMetric(props: {
  label: string;
  stats?: AccountQualityStatistics;
  unavailable?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div
            tabIndex={0}
            className="grid grid-cols-[3rem_1fr_1fr] gap-2 rounded-sm text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        }
      >
        <span className="text-muted-foreground">{props.label}</span>
        <span
          aria-label={`${props.label}短期：${props.unavailable ?? score(props.stats?.short)}`}
          className="tabular-nums"
        >
          {props.unavailable ?? score(props.stats?.short)}
        </span>
        <span
          aria-label={`${props.label}长期：${props.unavailable ?? score(props.stats?.long)}`}
          className="tabular-nums"
        >
          {props.unavailable ?? score(props.stats?.long)}
        </span>
      </TooltipTrigger>
      <TooltipContent role="tooltip" className="grid max-w-80 gap-2 text-xs">
        <strong>{props.label}统计</strong>
        <p>
          {props.label === "置信度"
            ? "模型行为检测一致率的 95% Wilson 下界；样本越少越保守，不代表模型身份的真实概率。不同模型和规则版本合并统计。"
            : "真实请求与主动探活的成功率；跨分组的同一证据只计一次，疑似空回复计为失败。"}
        </p>
        {(["short", "long"] as const).map((key) => {
          const window = props.stats?.[key];
          return (
            <p key={key}>
              {key === "short" ? "短期 24 小时" : "长期 30 天"}：{window?.samples ?? 0} 个样本，
              {window?.passed ?? 0} 个通过，{window?.failed ?? 0} 个失败，
              {window?.inconclusive ?? 0} 个无结论。
            </p>
          );
        })}
        <p>无结论不计入分母；仅统计当前保留的记录，历史清理可能缩短覆盖范围。</p>
        {props.stats?.evaluated_at ? (
          <p>统计时间：{new Date(props.stats.evaluated_at).toLocaleString("zh-CN")}</p>
        ) : null}
      </TooltipContent>
    </Tooltip>
  );
}

export function AccountQualityCell(props: { account: AccountStatus }) {
  const status = accountCheckStatus(props.account);
  let unavailable: string | undefined;
  if (props.account.model_check_status === "loading") unavailable = "读取中";
  if (props.account.model_check_status === "unavailable") unavailable = "读取失败";
  return (
    <div className="grid min-w-52 gap-1.5">
      <div className="grid grid-cols-[3rem_1fr_1fr] gap-2 text-[11px] text-muted-foreground">
        <span>{status.label}</span>
        <span>短期 · 24h</span>
        <span>长期 · 30天</span>
      </div>
      <QualityMetric
        label="置信度"
        stats={props.account.model_check?.confidence}
        unavailable={unavailable}
      />
      <QualityMetric label="稳定性" stats={props.account.stability} />
    </div>
  );
}
