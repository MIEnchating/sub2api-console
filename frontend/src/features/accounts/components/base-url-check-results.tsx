import { Play, RefreshCw } from "lucide-react";
import { useMemo, useState } from "react";

import type { AccountStatus } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { SearchField } from "@/components/data-table/search-field";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { PageActions } from "@/components/page-actions";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { TableOverflowTooltip } from "@/components/ui/table-overflow-tooltip";
import { useClientPagination } from "@/hooks/use-client-pagination";

import { accountIdentityMeta } from "../lib/account-labels";
import { accountBaseURLPresentation } from "./account-pool-cells";

type RepairKind = "base_url" | "upstream_host";

export type BaseURLCheckResultsProps = {
  accounts: AccountStatus[];
  running: boolean;
  repairing: boolean;
  repairingAccountId: string | null;
  onRerun: () => void;
  onRepair: (accountId: string, kind: RepairKind) => void;
};

const resultPriority: Record<NonNullable<AccountStatus["base_url_check"]>, number> = {
  official_mismatch: 0,
  invalid: 1,
  unknown: 2,
  unchecked: 3,
  different_allowed: 4,
  matched: 5,
};

function repairKind(account: AccountStatus): RepairKind | null {
  if (account.upstream_host_repairable) return "upstream_host";
  if (
    account.base_url_check === "official_mismatch" &&
    account.base_url_source === "platform_default" &&
    account.upstream_base_url
  ) {
    return "base_url";
  }
  return null;
}

function matchesSearch(account: AccountStatus, query: string): boolean {
  const normalized = query.trim().toLocaleLowerCase();
  if (!normalized) return true;
  return [
    account.id,
    account.name,
    account.upstream_host,
    account.recorded_upstream_host,
    account.upstream_base_url,
    account.base_url,
    account.base_url_check_reason,
    accountBaseURLPresentation(account).label,
    ...account.groups,
  ].some((value) => value?.toLocaleLowerCase().includes(normalized));
}

export function BaseURLCheckResults(props: BaseURLCheckResultsProps) {
  const [search, setSearch] = useState("");
  const filteredAccounts = useMemo(
    () =>
      [...props.accounts]
        .sort(
          (left, right) =>
            (resultPriority[left.base_url_check ?? "unknown"] ?? 5) -
              (resultPriority[right.base_url_check ?? "unknown"] ?? 5) ||
            left.name.localeCompare(right.name),
        )
        .filter((account) => matchesSearch(account, search)),
    [props.accounts, search],
  );
  const pagination = useClientPagination(filteredAccounts);

  return (
    <div
      className="grid h-full min-h-0 min-w-0 grid-rows-[auto_auto_minmax(0,1fr)] gap-3"
      data-testid="base-url-check-results"
    >
      <PageActions>
        <Button variant="outline" size="sm" disabled={props.running} onClick={props.onRerun}>
          <Play size={15} aria-hidden="true" />
          重新运行
        </Button>
      </PageActions>

      <TableFilterToolbar>
        <SearchField
          value={search}
          onChange={(value) => {
            setSearch(value);
            pagination.setCurrentPage(1);
          }}
          placeholder="搜索账号、ID、Host 或 Base URL"
        />
      </TableFilterToolbar>

      <DataTablePanel className="flex-1">
        <Table
          overflowTooltip={false}
          containerClassName="min-h-0 flex-1 overflow-auto"
          className="min-w-[1120px] table-fixed"
        >
          <TableHeader>
            <TableRow>
              <TableHead className="w-[210px]">账号</TableHead>
              <TableHead className="w-[170px]">归属上游 Host</TableHead>
              <TableHead className="w-[190px]">上游访问地址</TableHead>
              <TableHead className="w-[205px]">账号 Base URL</TableHead>
              <TableHead className="w-[255px]">判断结果</TableHead>
              <TableHead className="w-[130px] text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {pagination.visibleItems.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-muted-foreground h-24 text-center">
                  {search ? "没有匹配的账号" : "当前没有账号"}
                </TableCell>
              </TableRow>
            ) : (
              pagination.visibleItems.map((account) => {
                const currentRepairKind = repairKind(account);
                const presentation = accountBaseURLPresentation(account);
                return (
                  <TableRow key={account.id}>
                    <TableCell className="align-top whitespace-normal">
                      <div className="grid min-w-0 gap-1">
                        <TableOverflowTooltip content={account.name} className="font-semibold">
                          {account.name}
                        </TableOverflowTooltip>
                        <span className="text-muted-foreground text-xs">
                          {accountIdentityMeta(account)}
                        </span>
                        <TableOverflowTooltip
                          content={account.groups.length ? account.groups.join("、") : "未分组"}
                          className="text-muted-foreground text-xs"
                        >
                          分组：
                          {account.groups.length ? account.groups.join("、") : "未分组"}
                        </TableOverflowTooltip>
                      </div>
                    </TableCell>
                    <TableCell className="align-top whitespace-normal">
                      <TableOverflowTooltip
                        content={account.upstream_host ?? "未记录"}
                        className="font-medium"
                      >
                        {account.upstream_host ?? "未记录"}
                      </TableOverflowTooltip>
                      {account.upstream_host_repairable ? (
                        <TableOverflowTooltip
                          content={`账号记录：${account.recorded_upstream_host ?? "未记录"}`}
                          className="text-warning mt-1 text-xs"
                        >
                          账号记录：{account.recorded_upstream_host ?? "未记录"}
                        </TableOverflowTooltip>
                      ) : null}
                    </TableCell>
                    <TableCell className="align-top whitespace-normal">
                      <TableOverflowTooltip content={account.upstream_base_url ?? "未登记"}>
                        {account.upstream_base_url ?? "未登记"}
                      </TableOverflowTooltip>
                    </TableCell>
                    <TableCell className="align-top whitespace-normal">
                      <TableOverflowTooltip
                        content={account.base_url ?? "未返回"}
                        className="font-medium"
                      >
                        {account.base_url ?? "未返回"}
                      </TableOverflowTooltip>
                      {account.base_url_source === "platform_default" ? (
                        <span className="text-muted-foreground mt-1 block truncate text-xs">
                          Sub2API 平台默认地址
                        </span>
                      ) : null}
                    </TableCell>
                    <TableCell className="align-top whitespace-normal">
                      <div className="grid min-w-0 gap-1.5">
                        <StatusBadge label={presentation.label} variant={presentation.variant} />
                        <span className="text-muted-foreground line-clamp-2 text-xs leading-5 whitespace-normal">
                          {account.base_url_check_reason ?? "没有校验说明"}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="align-top text-right">
                      {currentRepairKind ? (
                        <Button
                          size="sm"
                          variant="outline"
                          className="w-[120px] justify-center"
                          disabled={props.repairing}
                          onClick={() => props.onRepair(account.id, currentRepairKind)}
                        >
                          <RefreshCw
                            size={14}
                            className={
                              props.repairingAccountId === account.id && props.repairing
                                ? "animate-spin"
                                : ""
                            }
                            aria-hidden="true"
                          />
                          {props.repairingAccountId === account.id && props.repairing
                            ? "修复中"
                            : currentRepairKind === "base_url"
                              ? "修复并恢复"
                              : "修复归属"}
                        </Button>
                      ) : (
                        <span className="text-muted-foreground text-xs">—</span>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
        {filteredAccounts.length > 0 ? (
          <DataTablePagination
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            totalItems={filteredAccounts.length}
            pageSize={pagination.pageSize}
            pageSizes={[10, 20, 50, 100]}
            onPageChange={pagination.setCurrentPage}
            onPageSizeChange={pagination.setPageSize}
          />
        ) : null}
      </DataTablePanel>
    </div>
  );
}
