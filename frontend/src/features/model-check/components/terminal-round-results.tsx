import type { ReactElement } from "react";
import type { TerminalContinuityRound } from "@/api";
import { terminalVerdicts } from "../constants";
export function TerminalRoundResults(props: {
  rounds?: TerminalContinuityRound[];
  invalidRounds?: boolean;
}): ReactElement | null {
  if (!props.rounds?.length && !props.invalidRounds) return null;
  return (
    <section aria-label="逐轮检测结果" className="space-y-3 border-t pt-3">
      <h3 className="text-sm font-medium">逐轮检测结果（{props.rounds?.length ?? 0} 轮）</h3>
      {props.invalidRounds ? (
        <p className="text-sm text-muted-foreground">部分逐轮结果格式无效，已保留可读取的结果。</p>
      ) : null}
      {props.rounds?.map((round) => (
        <details key={round.request_id} className="rounded border p-3 text-sm">
          <summary className="cursor-pointer">
            第 {round.round} 轮 · {terminalVerdicts[round.verdict].label} ·{" "}
            {(round.duration_ms / 1000).toFixed(1)} 秒
          </summary>
          <p className="mt-2 whitespace-pre-wrap wrap-anywhere">
            {round.error || round.response || "未返回回答"}
          </p>
          <p className="mt-2 text-xs text-muted-foreground wrap-anywhere">
            请求 ID：{round.request_id}
          </p>
        </details>
      ))}
    </section>
  );
}
