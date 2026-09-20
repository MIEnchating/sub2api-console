import { useState, type ReactElement } from "react";
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
import { accountStatusLabels, subscriptionLabels, workbenchKeys } from "../constants";
import { accountStatus, matchesAccount } from "../lib/accounts";
import type { WorkbenchAccount } from "../types";
import { WorkbenchToolbar, WorkbenchEmptyState } from "./workbench-section";
import { AccountDetails } from "./account-details";

export function AccountList(): ReactElement {
  const query = useQuery({ queryKey: workbenchKeys.accounts, queryFn: api.workbenchAccounts });
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [group, setGroup] = useState("all");
  const [details, setDetails] = useState<WorkbenchAccount | null>(null);
  const accounts = query.data ?? [];
  const groups = Array.from(
    new Map(accounts.flatMap((account) => account.groups).map((item) => [item.id, item])).values(),
  );
  const selectedGroup = groups.some((item) => item.id === group) ? group : "all";
  const filtered = accounts.filter((account) =>
    matchesAccount(account, { query: search, status, group: selectedGroup }),
  );
  return (
    <section aria-label="线上账号列表" className="grid min-w-0 gap-4">
      <WorkbenchToolbar
        meta={
          query.data && (
            <Badge variant="secondary">
              {filtered.length} / {accounts.length}
            </Badge>
          )
        }
        actions={
          <Button
            variant="outline"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw aria-hidden="true" />
            刷新列表
          </Button>
        }
      />
      <div className="grid min-w-0 grid-cols-2 gap-2 rounded-xl border bg-card p-3 md:grid-cols-[minmax(0,1fr)_10rem_12rem]">
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
            onChange={(event) => setSearch(event.target.value)}
          />
        </div>
        <Select value={status} onValueChange={(value) => setStatus(value ?? "all")}>
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
        <Select value={selectedGroup} onValueChange={(value) => setGroup(value ?? "all")}>
          <SelectTrigger aria-label="筛选账号分组">
            <SelectValue>
              {selectedGroup === "all"
                ? "全部分组"
                : groups.find((item) => item.id === selectedGroup)?.name ||
                  `分组 #${selectedGroup}`}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部分组</SelectItem>
            {groups.map((item) => (
              <SelectItem key={item.id} value={item.id}>
                {item.name || `分组 #${item.id}`}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
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
        <Table
          aria-label="账号列表"
          actionColumn
          uniformTextSize={false}
          className="min-w-[800px]"
          containerClassName="overflow-auto rounded-xl border bg-card"
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
            {filtered.map((account) => (
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
                    onClick={() => setDetails(account)}
                    aria-label={`查看 ${account.name || account.id} 配置`}
                  >
                    查看
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      {details && <AccountDetails account={details} onClose={() => setDetails(null)} />}
    </section>
  );
}
