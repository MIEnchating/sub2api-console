import { useRef, useState } from "react";
import { ChartNoAxesColumnIncreasing } from "lucide-react";

import type { UpstreamAccountRequestUsage } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { DataTablePagination } from "@/components/data-table/pagination";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatUsageAmount } from "../lib/usage-totals";

type Props = {
  name: string;
  spending?: { total: string; started_at: string; updated_at: string } | null;
  requestCount?: number;
  usagePending?: boolean;
  onRetryUsage?: () => void;
  requestWindowDays: number;
  accountRequestUsage: UpstreamAccountRequestUsage[];
};

export function UpstreamUsageSummary(props: Props) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const title = useRef<HTMLHeadingElement>(null);
  const amount = props.spending ? formatUsageAmount(props.spending.total) : "待同步";
  return (
    <>
      <div className="flex min-w-0 items-center gap-2 whitespace-nowrap" aria-label="上游使用汇总">
        <span className="text-muted-foreground shrink-0">
          消费{" "}
          <Tooltip>
            <TooltipTrigger render={<span tabIndex={0} className="text-foreground tabular-nums" />}>
              {amount}
            </TooltipTrigger>
            <TooltipContent>{props.spending?.total ?? "待同步"}</TooltipContent>
          </Tooltip>
        </span>
        <span aria-hidden="true" className="text-muted-foreground/50">
          ·
        </span>
        {props.requestCount === undefined ? (
          <UsageLoading pending={props.usagePending} onRetry={props.onRetryUsage} />
        ) : (
          <Tooltip>
            <TooltipTrigger
              render={<span tabIndex={0} className="text-muted-foreground truncate tabular-nums" />}
            >
              {props.requestCount.toLocaleString("zh-CN")} 次
            </TooltipTrigger>
            <TooltipContent>近 {props.requestWindowDays} 天请求数</TooltipContent>
          </Tooltip>
        )}
        <Button
          ref={trigger}
          type="button"
          variant="ghost"
          size="icon"
          aria-label={`查看${props.name}的账号使用率`}
          aria-haspopup="dialog"
          disabled={props.requestCount === undefined}
          onClick={() => setOpen(true)}
        >
          <ChartNoAxesColumnIncreasing aria-hidden="true" />
        </Button>
      </div>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent width="progress" height="large" initialFocus={title} finalFocus={trigger}>
          <DialogHeader>
            <DialogTitle ref={title} tabIndex={-1} className="outline-none">
              {props.name} · 使用统计
            </DialogTitle>
            <DialogDescription>
              近 {props.requestWindowDays} 天请求占比 = 账号请求数 ÷ 此上游总请求数。
            </DialogDescription>
          </DialogHeader>
          <DialogBody>
            <dl className="bg-muted/40 mb-4 grid grid-cols-1 gap-4 rounded-lg border p-4 sm:grid-cols-2">
              <div className="min-w-0">
                <dt className="text-muted-foreground">累计消费</dt>
                <dd
                  className="mt-1 text-xl font-semibold break-all tabular-nums"
                  aria-label={props.spending?.total}
                >
                  <Tooltip>
                    <TooltipTrigger render={<span tabIndex={0} />}>{amount}</TooltipTrigger>
                    <TooltipContent>{props.spending?.total ?? "待同步"}</TooltipContent>
                  </Tooltip>
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-muted-foreground">近 {props.requestWindowDays} 天请求</dt>
                <dd className="mt-1 text-xl font-semibold break-all tabular-nums">
                  {props.requestCount?.toLocaleString("zh-CN")}
                </dd>
              </div>
            </dl>
            <p className="text-muted-foreground mb-4 text-xs">
              消费自首次同步起按余额下降量累计，已按充值比例换算；同步间隔内的充值与消费无法分别识别。
            </p>
            <AccountUsageTable accounts={props.accountRequestUsage} />
          </DialogBody>
        </DialogContent>
      </Dialog>
    </>
  );
}

function UsageLoading(props: { pending?: boolean; onRetry?: () => void }) {
  if (props.pending) return <ContentLoading compact label="正在读取用量" />;
  if (props.onRetry) return <ContentRetry compact onRetry={props.onRetry} />;
  return null;
}

function AccountUsageTable(props: { accounts: UpstreamAccountRequestUsage[] }) {
  const pagination = useClientPagination(props.accounts);
  if (props.accounts.length === 0) {
    return <p className="text-muted-foreground py-6 text-center">暂无绑定账号</p>;
  }
  return (
    <div className="min-w-0 overflow-hidden rounded-lg border">
      <Table aria-label="账号使用率" overflowTooltip={false} className="table-fixed">
        <TableHeader>
          <TableRow>
            <TableHead>账号</TableHead>
            <TableHead className="w-24 text-right">请求数</TableHead>
            <TableHead className="w-24 text-right">使用率</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {pagination.visibleItems.map((account) => (
            <TableRow key={account.account_id}>
              <TableCell>
                <Tooltip>
                  <TooltipTrigger render={<span tabIndex={0} className="block truncate" />}>
                    {account.account_name}
                  </TooltipTrigger>
                  <TooltipContent>
                    {account.account_name} · ID {account.account_id}
                  </TooltipContent>
                </Tooltip>
                <span className="text-muted-foreground block truncate text-xs">
                  ID {account.account_id}
                </span>
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {account.request_count.toLocaleString("zh-CN")}
              </TableCell>
              <TableCell className="text-right font-medium tabular-nums">
                {account.share.toFixed(2)}%
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <DataTablePagination
        currentPage={pagination.currentPage}
        totalPages={pagination.totalPages}
        totalItems={props.accounts.length}
        pageSize={pagination.pageSize}
        onPageChange={pagination.setCurrentPage}
        onPageSizeChange={pagination.setPageSize}
      />
    </div>
  );
}
