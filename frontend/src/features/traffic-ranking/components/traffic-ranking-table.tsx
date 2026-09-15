import { ArrowDown, ArrowUp } from "lucide-react";
import type { ReactElement } from "react";

import type { TrafficRanking, TrafficRankingRow as RankingRow, TrafficRankingSort } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { TableEmptyState } from "@/components/data-table/empty-state";
import { Table, TableBody, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { cn } from "@/lib/utils";

import { TrafficRankingRow } from "./traffic-ranking-row";

type TrafficRankingTableProps = {
  rows: RankingRow[];
  bucket: TrafficRanking["bucket"];
  sortBy: TrafficRankingSort;
  failed: boolean;
  pending: boolean;
  onRetry: () => void;
};

function MetricHeading(props: {
  label: string;
  detail: string;
  active?: boolean;
  ascending?: boolean;
  className: string;
  title?: string;
}): ReactElement {
  const direction = props.ascending ? "ascending" : "descending";
  const Icon = props.ascending ? ArrowUp : ArrowDown;
  return (
    <TableHead
      scope="col"
      title={props.title}
      aria-sort={props.active ? direction : undefined}
      className={cn("h-14 px-3 text-right", props.className)}
    >
      <div
        className={cn(
          "flex items-center justify-end gap-1",
          props.active && "text-secondary-foreground",
        )}
      >
        {props.active && <Icon aria-hidden="true" className="size-3.5" />}
        {props.label}
      </div>
      <div className="text-muted-foreground mt-0.5 text-xs font-normal">{props.detail}</div>
    </TableHead>
  );
}

export function TrafficRankingTable(props: TrafficRankingTableProps): ReactElement {
  let content = props.rows.map((row) => (
    <TrafficRankingRow key={row.account_id} row={row} bucket={props.bucket} />
  ));
  if (props.failed) {
    content = [
      <TableEmptyState key="retry" columns={8}>
        <ContentRetry pending={props.pending} onRetry={props.onRetry} />
      </TableEmptyState>,
    ];
  } else if (props.rows.length === 0) {
    content = [
      <TableEmptyState key="empty" columns={8}>
        当前范围没有匹配的账号流量
      </TableEmptyState>,
    ];
  }
  return (
    <div className="min-h-0 min-w-0 flex-1 overflow-hidden">
      <Table
        aria-label="账号流量排行"
        uniformTextSize={false}
        containerClassName="h-full overflow-auto overscroll-contain"
        className="min-w-[1184px] sm:min-w-[1248px]"
      >
        <TableHeader className="[&_th:first-child]:z-20">
          <TableRow>
            <TableHead
              scope="col"
              className="left-0 h-14 w-48 px-3 shadow-[1px_0_0_var(--border)] sm:w-64"
            >
              排名 / 账号
            </TableHead>
            <MetricHeading
              label="请求数"
              detail="流量占比"
              active={props.sortBy === "traffic"}
              className="w-36"
            />
            <MetricHeading
              label="稳定性"
              detail="评分 / 状态"
              active={props.sortBy === "stability"}
              className="w-28"
            />
            <MetricHeading
              label="成功率"
              detail="成功 / 失败"
              active={props.sortBy === "success_rate"}
              className="w-32"
            />
            <MetricHeading
              label="响应延迟"
              detail="平均 / P95"
              active={props.sortBy === "latency"}
              ascending
              className="w-32"
            />
            <MetricHeading label="活跃时段" detail="最后流量" className="w-40" />
            <MetricHeading label="Token 用量" detail="输入 / 输出" className="w-40" />
            <MetricHeading
              label="缓存占比"
              detail="占输入侧用量"
              title="读取、写入分别占输入侧用量的比例；输入侧用量 = 输入 + 缓存读取 + 缓存写入，不含输出。"
              className="w-40"
            />
          </TableRow>
        </TableHeader>
        <TableBody>{content}</TableBody>
      </Table>
    </div>
  );
}
