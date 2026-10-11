import { useCallback, useEffect, useMemo, useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, Search, Users } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableHeader,
  TableHead,
  TableRow,
  TableBody,
  TableCell,
} from "@/components/ui/table";
import { ContentRetry } from "@/components/content-retry";
import { Badge } from "@/components/ui/badge";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { DataTablePagination } from "@/components/data-table/pagination";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { accountStatusLabels, subscriptionLabels, workbenchKeys } from "../constants";
import { accountStatus, createAccountSearchIndex, filterAccountSearchIndex } from "../lib/accounts";
import type { WorkbenchAccount } from "../types";
import { WorkbenchToolbar, WorkbenchEmptyState } from "./workbench-section";
import { AccountDetails } from "./account-details";
import { AccountGroupFilter } from "./account-group-filter";
import { RememberedAccountList } from "./remembered-account-list";

const emptyAccounts: WorkbenchAccount[] = [];

export function AccountList(props: { onStarted?: () => void }): ReactElement {
  const query = useQuery({
    queryKey: workbenchKeys.accounts,
    queryFn: ({ signal }) => api.workbenchAccounts(signal),
  });
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [group, setGroup] = useState("all");
  const [detailsID, setDetailsID] = useState<string | null>(null);
  const [showRemembered, setShowRemembered] = useState(false);
  const accounts = query.data ?? emptyAccounts;
  const details = useMemo(
    () => (detailsID === null ? undefined : accounts.find((account) => account.id === detailsID)),
    [accounts, detailsID],
  );
  useEffect(() => {
    if (query.data && detailsID !== null && !details) setDetailsID(null);
  }, [query.data, detailsID, details]);
  const searchIndex = useMemo(() => createAccountSearchIndex(accounts), [accounts]);
  const filtered = useMemo(
    () => filterAccountSearchIndex(searchIndex, { query: search, status, group }),
    [searchIndex, search, status, group],
  );
  const pagination = useClientPagination(filtered);
  const setCurrentPage = pagination.setCurrentPage;
  const changeGroup = useCallback(
    (value: string): void => {
      setGroup(value);
      setCurrentPage(1);
    },
    [setCurrentPage],
  );
  return (
    <section aria-label="线上账号列表" className="grid min-w-0 gap-4">
      <WorkbenchToolbar
        description="查看线上账号状态、分组与连接配置。"
        meta={
          query.data && (
            <Badge variant="secondary">
              显示 {filtered.length} / {accounts.length} 个账号
            </Badge>
          )
        }
        actions={
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              <RefreshCw aria-hidden="true" />
              刷新列表
            </Button>
            <Button
              variant="outline"
              aria-expanded={showRemembered}
              aria-controls="remembered-accounts"
              onClick={() => setShowRemembered((current) => !current)}
            >
              {showRemembered ? "隐藏已记住账号" : "查看已记住账号"}
            </Button>
          </div>
        }
      >
        <div className="grid min-w-0 grid-cols-2 gap-2 md:grid-cols-[minmax(0,1fr)_10rem_12rem]">
          <div className="relative col-span-2 min-w-0 md:col-span-1">
            <Search
              aria-hidden="true"
              className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground"
            />
            <Input
              type="search"
              aria-label="搜索账号"
              placeholder="搜索名称、邮箱、ID 或分组"
              className="pl-9"
              value={search}
              onChange={(event) => {
                setSearch(event.target.value);
                pagination.setCurrentPage(1);
              }}
            />
          </div>
          <Select
            value={status}
            onValueChange={(value) => {
              setStatus(value ?? "all");
              pagination.setCurrentPage(1);
            }}
          >
            <SelectTrigger aria-label="筛选账号状态">
              <SelectValue>
                {status === "all" ? "全部状态" : accountStatusLabels[status] || "状态待确认"}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部状态</SelectItem>
              {Object.entries(accountStatusLabels)
                .filter(([id]) => id !== "unknown")
                .map(([id, label]) => (
                  <SelectItem key={id} value={id}>
                    {label}
                  </SelectItem>
                ))}
            </SelectContent>
          </Select>
          <AccountGroupFilter accounts={accounts} value={group} onChange={changeGroup} />
        </div>
      </WorkbenchToolbar>
      {showRemembered && <RememberedAccountList onStarted={props.onStarted} />}
      {query.isPending && (
        <div aria-label="正在读取账号" aria-busy="true" className="grid gap-3">
          {[0, 1, 2, 3].map((id) => (
            <Skeleton key={id} className="h-12 w-full" />
          ))}
        </div>
      )}
      {!query.data && query.isError && (
        <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
      )}
      {query.data && filtered.length === 0 && (
        <WorkbenchEmptyState
          icon={Users}
          title={accounts.length ? "没有匹配的账号" : "当前站点暂无 OpenAI OAuth 账号"}
          description={
            accounts.length
              ? "调整搜索词或筛选条件后重试。"
              : "从导入账号页添加账号后，可在这里查看配置。"
          }
        />
      )}
      {filtered.length > 0 && (
        <DataTablePanel>
          <Table
            aria-label="账号列表"
            actionColumn
            uniformTextSize={false}
            className="min-w-[800px]"
            containerClassName="overflow-auto"
          >
            <TableHeader>
              <TableRow>
                <TableHead className="w-56">账号</TableHead>
                <TableHead className="w-24">状态</TableHead>
                <TableHead className="w-32">订阅</TableHead>
                <TableHead className="w-40">分组</TableHead>
                <TableHead>连接</TableHead>
                <TableHead className="w-20">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pagination.visibleItems.map((account) => (
                <TableRow key={account.id}>
                  <TableCell
                    className="max-w-72 whitespace-normal wrap-anywhere"
                    overflowTooltip={false}
                  >
                    <strong className="font-medium">
                      {account.name || account.email || `账号 ${account.id}`}
                    </strong>
                    {account.email !== account.name && (
                      <div className="text-xs text-muted-foreground">{account.email}</div>
                    )}
                    <div className="text-xs text-muted-foreground">#{account.id}</div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary">{accountStatusLabels[accountStatus(account)]}</Badge>
                    <div className="text-xs text-muted-foreground">
                      {account.schedulable ? "可调度" : "暂停调度"}
                    </div>
                  </TableCell>
                  <TableCell>
                    {subscriptionLabels[account.plan] || (account.plan ? "其他订阅" : "自动识别")}
                  </TableCell>
                  <TableCell
                    className="max-w-56 whitespace-normal wrap-anywhere"
                    overflowTooltip={false}
                  >
                    {account.groups.map((item) => item.name || `分组 #${item.id}`).join("、") ||
                      "未分组"}
                  </TableCell>
                  <TableCell className="whitespace-normal wrap-anywhere" overflowTooltip={false}>
                    {account.proxy_name || "直连"}
                    <div className="text-xs text-muted-foreground">
                      并发 {account.concurrency || "未提供"} · 倍率{" "}
                      {account.rate_multiplier || "未提供"}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      onClick={() => setDetailsID(account.id)}
                      aria-label={`查看 ${account.name || account.id} 配置`}
                    >
                      查看
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <DataTablePagination
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            totalItems={filtered.length}
            pageSize={pagination.pageSize}
            onPageChange={pagination.setCurrentPage}
            onPageSizeChange={pagination.setPageSize}
          />
        </DataTablePanel>
      )}
      {details && <AccountDetails account={details} onClose={() => setDetailsID(null)} />}
    </section>
  );
}
