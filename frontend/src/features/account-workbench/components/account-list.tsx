import { useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
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
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          账号列表{" "}
          {query.data && (
            <Badge variant="secondary">
              {filtered.length} / {accounts.length}
            </Badge>
          )}
        </h2>
        <Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>
          <RefreshCw aria-hidden="true" />
          刷新列表
        </Button>
      </div>
      <div className="grid min-w-0 grid-cols-2 gap-2 rounded-lg border bg-muted/20 p-3 md:grid-cols-[minmax(0,1fr)_10rem_12rem]">
        <Input
          type="search"
          aria-label="搜索账号"
          placeholder="搜索名称、邮箱、ID 或分组"
          className="col-span-2 md:col-span-1"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
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
        <p className="rounded-lg border border-dashed bg-muted/10 px-4 py-12 text-center text-sm text-muted-foreground">
          {accounts.length ? "没有匹配的账号" : "当前站点暂无 OpenAI OAuth 账号"}
        </p>
      )}
      {filtered.length > 0 && (
        <Table
          aria-label="账号列表"
          className="min-w-[850px]"
          containerClassName="overflow-auto rounded-lg border"
        >
          <TableHeader>
            <TableRow>
              {["账号", "状态", "订阅", "分组", "连接", "操作"].map((label) => (
                <TableHead key={label}>{label}</TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((account) => (
              <TableRow key={account.id}>
                <TableCell
                  className="max-w-72 whitespace-normal wrap-anywhere"
                  overflowTooltip={false}
                >
                  <strong>{account.name || account.email || `账号 ${account.id}`}</strong>
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
                <TableCell>
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
