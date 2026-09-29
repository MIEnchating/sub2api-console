import { Fragment, useEffect, useId, useMemo, useRef, useState, type ReactElement } from "react";
import { ChevronDown, ChevronRight, Trash2 } from "lucide-react";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { DataTablePagination } from "@/components/data-table/pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { aggregateGroupHistory, groupHistoryTime } from "../lib/group-history";
import { upstreamRateLabels } from "../lib/upstream-rate-labels";
import {
  UpstreamGroupHistoryAction,
  latestHistoryGroupChanges,
  type UpstreamGroupHistoryProps,
} from "./upstream-group-history";

export function UpstreamGroupHistoryOverview(
  props: UpstreamGroupHistoryProps & {
    upstreams: NonNullable<UpstreamGroupHistoryProps["upstreams"]>;
  },
): ReactElement {
  const detailsID = useId();
  const [expanded, setExpanded] = useState<Set<string>>(
    () => new Set(props.initialExpandedUpstreamID ? [props.initialExpandedUpstreamID] : []),
  );
  const summaries = useMemo(() => aggregateGroupHistory(props.rows), [props.rows]);
  const latestChanges = useMemo(() => latestHistoryGroupChanges(props.rows), [props.rows]);
  const upstreams = useMemo(
    () => new Map(props.upstreams.map((item) => [item.upstream_id, item])),
    [props.upstreams],
  );
  const pagination = useClientPagination(summaries);
  const restored = useRef(false);
  useEffect(() => {
    if (restored.current || !props.initialExpandedUpstreamID || summaries.length === 0) return;
    const index = summaries.findIndex(
      (summary) => summary.upstreamID === props.initialExpandedUpstreamID,
    );
    if (index >= 0) pagination.setCurrentPage(Math.floor(index / pagination.pageSize) + 1);
    restored.current = true;
  }, [props.initialExpandedUpstreamID, summaries, pagination.pageSize, pagination.setCurrentPage]);
  function toggleUpstream(upstreamID: string): void {
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(upstreamID)) next.delete(upstreamID);
      else next.add(upstreamID);
      return next;
    });
  }
  return (
    <DataTablePanel className="h-full flex-1">
      <p className="text-muted-foreground shrink-0 px-3 py-2 text-sm">
        最近 {props.rows.length} 条变化 · {summaries.length} 个上游
      </p>
      <Table
        aria-label="按上游汇总分组变化"
        className="min-w-[640px]"
        containerClassName="min-h-0 flex-1 overflow-auto"
      >
        <TableHeader>
          <TableRow>
            <TableHead className="w-12">
              <span className="sr-only">明细</span>
            </TableHead>
            <TableHead>上游</TableHead>
            <TableHead className="w-20">新增</TableHead>
            <TableHead className="w-20">删除</TableHead>
            <TableHead className="w-48">最近变化</TableHead>
            {props.onClearUpstream && <TableHead className="w-28">操作</TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {!summaries.length && (
            <TableRow>
              <TableCell
                colSpan={props.onClearUpstream ? 6 : 5}
                className="text-muted-foreground h-32 text-center"
              >
                暂无上游分组变化记录
              </TableCell>
            </TableRow>
          )}
          {pagination.visibleItems.map((summary) => {
            const upstream = upstreams.get(summary.upstreamID);
            const name = upstream?.name || upstream?.host || summary.upstreamID;
            const open = expanded.has(summary.upstreamID);
            const label = `${open ? "收起" : "展开"} ${name} 的变化明细`;
            const panelID = `${detailsID}-${summary.upstreamID}`;
            return (
              <Fragment key={summary.upstreamID}>
                <TableRow
                  className="cursor-pointer"
                  aria-expanded={open}
                  onClick={() => toggleUpstream(summary.upstreamID)}
                >
                  <TableCell overflowTooltip={false}>
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={label}
                            aria-expanded={open}
                            aria-controls={open ? panelID : undefined}
                            onClick={(event) => {
                              event.stopPropagation();
                              toggleUpstream(summary.upstreamID);
                            }}
                          >
                            {open ? (
                              <ChevronDown aria-hidden="true" />
                            ) : (
                              <ChevronRight aria-hidden="true" />
                            )}
                          </Button>
                        }
                      />
                      <TooltipContent role="tooltip">{label}</TooltipContent>
                    </Tooltip>
                  </TableCell>
                  <TableCell>
                    <div className="grid min-w-0 gap-0.5">
                      <span className="truncate font-medium">{name}</span>
                      {upstream?.host && (
                        <span className="text-muted-foreground truncate text-xs">
                          {upstream.host}
                        </span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell aria-label={`新增 ${summary.added} 次`}>
                    <Badge variant="outline">{summary.added}</Badge>
                  </TableCell>
                  <TableCell aria-label={`删除 ${summary.removed} 次`}>
                    <Badge variant={summary.removed ? "destructive" : "outline"}>
                      {summary.removed}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground tabular-nums">
                    {groupHistoryTime(summary.latestAt)}
                  </TableCell>
                  {props.onClearUpstream && (
                    <TableCell overflowTooltip={false}>
                      <Button
                        variant="outline"
                        aria-label={`清除 ${name} 的变化记录`}
                        onClick={(event) => {
                          event.stopPropagation();
                          props.onClearUpstream?.(summary.upstreamID, name);
                        }}
                      >
                        <Trash2 aria-hidden="true" />
                        清除变化
                      </Button>
                    </TableCell>
                  )}
                </TableRow>
                {open && (
                  <TableRow>
                    <TableCell
                      colSpan={props.onClearUpstream ? 6 : 5}
                      overflowTooltip={false}
                      className="whitespace-normal"
                    >
                      <section
                        id={panelID}
                        aria-label={`${name} 的变化明细`}
                        className="min-w-0 pl-10"
                      >
                        <ul className="divide-y">
                          {summary.changes.map((change) => (
                            <li
                              key={change.id}
                              className="grid grid-cols-[12rem_4rem_minmax(0,1fr)_auto] items-start gap-3 py-2 text-sm max-sm:grid-cols-[4rem_minmax(0,1fr)]"
                            >
                              <time
                                dateTime={change.changed_at}
                                className="text-muted-foreground tabular-nums max-sm:col-span-2"
                              >
                                {groupHistoryTime(change.changed_at)}
                              </time>
                              <Badge
                                variant={change.change_type === "added" ? "outline" : "destructive"}
                              >
                                {change.change_type === "added" ? "添加" : "删除"}
                              </Badge>
                              <span className="min-w-0 wrap-anywhere">
                                <span className="font-medium">{change.group_name}</span>
                                <span className="text-muted-foreground ml-2">
                                  #{change.group_id}
                                </span>
                                {change.change_type === "added" && (
                                  <Tooltip>
                                    <TooltipTrigger
                                      render={
                                        <span
                                          tabIndex={0}
                                          className="text-muted-foreground block tabular-nums"
                                        />
                                      }
                                    >
                                      {upstreamRateLabels.effectiveRate}：
                                      {change.effective_rate ?? "未计算"}
                                    </TooltipTrigger>
                                    <TooltipContent role="tooltip">
                                      最近同步并按充值比例换算后的分组倍率
                                    </TooltipContent>
                                  </Tooltip>
                                )}
                              </span>
                              <div className="max-sm:col-span-2 empty:hidden">
                                <UpstreamGroupHistoryAction
                                  change={change}
                                  latest={latestChanges.has(change.id)}
                                  upstreamAvailable={Boolean(upstream?.host)}
                                  bindingAuditItems={props.bindingAuditItems}
                                  onAddAccount={props.onAddAccount}
                                  onDeleteAccounts={props.onDeleteAccounts}
                                />
                              </div>
                            </li>
                          ))}
                        </ul>
                      </section>
                    </TableCell>
                  </TableRow>
                )}
              </Fragment>
            );
          })}
        </TableBody>
      </Table>
      {summaries.length > 0 && (
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={summaries.length}
          pageSize={pagination.pageSize}
          pageSizes={[10, 20, 50, 100]}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      )}
    </DataTablePanel>
  );
}
