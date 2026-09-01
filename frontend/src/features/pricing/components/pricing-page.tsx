import { Fragment, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeftRight,
  ArrowRight,
  ChevronDown,
  CircleDollarSign,
  CircleHelp,
  DatabaseBackup,
  Eye,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

import { api, type PricingConfig, type PricingDecision, type PricingGroup } from "@/api";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { DataTablePagination } from "@/components/data-table/pagination";
import { SearchField } from "@/components/data-table/search-field";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { taskPollInterval, taskStopsPolling } from "@/lib/task-state";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { cn } from "@/lib/utils";

function percent(value: number) {
  return `${(value * 100)
    .toFixed(2)
    .replace(/\.00$/, "")
    .replace(/(\.\d)0$/, "$1")}%`;
}

function groupIDsLabel(ids: string[], names: Map<string, string>) {
  if (ids.length === 0) return "未分组";
  return ids.map((id) => (names.get(id) ? `${names.get(id)}（#${id}）` : `分组 #${id}`)).join("、");
}

function decimal(value: number) {
  return value.toFixed(4).replace(/0+$/, "").replace(/\.$/, "");
}

function groupAccountCosts(groupID: string, decisions: PricingDecision[]) {
  const accounts = decisions.filter((decision) => decision.current_group_ids.includes(groupID));
  const values = [
    ...new Map(
      accounts.flatMap((decision) => {
        const value = Number(decision.cost_multiplier);
        if (!Number.isFinite(value) || value <= 0 || !decision.cost_multiplier) return [];
        return [[value, decision.cost_multiplier] as const];
      }),
    ).entries(),
  ]
    .sort(([left], [right]) => left - right)
    .map(([, label]) => label);
  return { accounts: accounts.length, values };
}

function GroupAccountCostCell(props: {
  groupID: string;
  decisions: PricingDecision[];
  onOpen: () => void;
}) {
  const summary = groupAccountCosts(props.groupID, props.decisions);
  if (summary.accounts === 0) return <span className="text-muted-foreground">无账号</span>;
  if (summary.values.length === 0) {
    return (
      <div className="grid gap-0.5">
        <button
          type="button"
          className="text-muted-foreground hover:text-foreground w-fit border-b border-dashed"
          aria-label={`查看分组 ${props.groupID} 的账号成本明细`}
          onClick={props.onOpen}
        >
          未记录
        </button>
        <span className="text-muted-foreground text-xs">{summary.accounts} 个账号</span>
      </div>
    );
  }
  const label =
    summary.values.length === 1
      ? summary.values[0]
      : `${summary.values[0]} - ${summary.values[summary.values.length - 1]}`;
  const content = `当前 ${summary.accounts} 个账号，账号成本共 ${summary.values.length} 档：${summary.values.join("、")}`;
  return (
    <div className="grid gap-0.5 tabular-nums">
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type="button"
              className="hover:text-primary w-fit border-b border-dashed font-medium"
              aria-label={`查看分组 ${props.groupID} 的账号成本明细`}
              onClick={props.onOpen}
            />
          }
        >
          {label}
        </TooltipTrigger>
        <TooltipContent className="max-w-sm whitespace-normal">{content}</TooltipContent>
      </Tooltip>
      <span className="text-muted-foreground text-xs">
        {summary.accounts} 个账号{summary.values.length > 1 ? ` · ${summary.values.length} 档` : ""}
      </span>
    </div>
  );
}

function AccountCostHeader(props: { range?: boolean } = {}) {
  return (
    <div className="flex items-center gap-1">
      <span>{props.range ? "账号成本范围" : "账号成本"}</span>
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type="button"
              className="text-muted-foreground hover:text-foreground inline-flex size-5 items-center justify-center"
              aria-label="账号成本说明"
            />
          }
        >
          <CircleHelp className="size-3.5" aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent className="max-w-sm whitespace-normal">
          账号从上游获取额度时的成本比例。例如 0.1 表示成本约为标准价格的 10%。
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

function accountCostValue(decision: PricingDecision) {
  const value = Number(decision.cost_multiplier);
  return Number.isFinite(value) && value > 0 ? value : null;
}

export function GroupAccountCostDetails(props: { groupID: string; decisions: PricingDecision[] }) {
  const [search, setSearch] = useState("");
  const accounts = useMemo(
    () =>
      props.decisions
        .filter((decision) => decision.current_group_ids.includes(props.groupID))
        .sort((left, right) => {
          const leftCost = accountCostValue(left);
          const rightCost = accountCostValue(right);
          if (leftCost === null && rightCost !== null) return 1;
          if (leftCost !== null && rightCost === null) return -1;
          if (leftCost !== null && rightCost !== null && leftCost !== rightCost) {
            return leftCost - rightCost;
          }
          return (left.account_name || left.account_id).localeCompare(
            right.account_name || right.account_id,
            "zh-CN",
          );
        }),
    [props.decisions, props.groupID],
  );
  const filteredAccounts = useMemo(() => {
    const query = search.trim().toLocaleLowerCase();
    if (!query) return accounts;
    return accounts.filter((decision) => {
      const cost = accountCostValue(decision);
      return [
        decision.account_name,
        decision.account_id,
        decision.platform,
        cost === null ? "未记录" : decision.cost_multiplier,
      ].some((value) => value?.toLocaleLowerCase().includes(query));
    });
  }, [accounts, search]);
  const pagination = useClientPagination(filteredAccounts);

  return (
    <div
      className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-3"
      data-testid="group-account-cost-details"
    >
      <TableFilterToolbar>
        <SearchField
          value={search}
          onChange={(value) => {
            setSearch(value);
            pagination.setCurrentPage(1);
          }}
          placeholder="搜索账号、ID、平台或成本档位"
        />
      </TableFilterToolbar>
      <DataTablePanel>
        <Table
          className="min-w-[40rem] table-fixed"
          containerClassName="min-h-0 flex-1 overflow-auto"
          overflowTooltip={false}
        >
          <TableHeader>
            <TableRow>
              <TableHead className="w-36">账号成本档位</TableHead>
              <TableHead>账号</TableHead>
              <TableHead className="w-40">平台</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {pagination.visibleItems.length === 0 ? (
              <TableRow>
                <TableCell colSpan={3} className="text-muted-foreground h-24 text-center">
                  {search ? "没有匹配的账号" : "该分组当前没有账号"}
                </TableCell>
              </TableRow>
            ) : (
              pagination.visibleItems.map((decision) => {
                const cost = accountCostValue(decision);
                return (
                  <TableRow key={decision.account_id}>
                    <TableCell className="font-medium tabular-nums">
                      {cost === null ? (
                        <span className="text-muted-foreground">未记录</span>
                      ) : (
                        decision.cost_multiplier
                      )}
                    </TableCell>
                    <TableCell className="whitespace-normal">
                      <div className="grid gap-0.5">
                        <span className="break-words font-medium">
                          {decision.account_name || `账号 ${decision.account_id}`}
                        </span>
                        <span className="text-muted-foreground text-xs">
                          #{decision.account_id}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>{decision.platform || "未记录"}</TableCell>
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

function pricingGroupChanges(decision: PricingDecision) {
  const current = new Set(decision.current_group_ids);
  const desired = new Set(decision.desired_group_ids);
  return {
    added: decision.desired_group_ids.filter((groupID) => !current.has(groupID)),
    removed: decision.current_group_ids.filter((groupID) => !desired.has(groupID)),
  };
}

function pricingGroupTransition(decision: PricingDecision, config: PricingConfig) {
  const changes = pricingGroupChanges(decision);
  const setByGroup = new Map<string, number>();
  config.exchange_group_sets.forEach((groupSet, setIndex) => {
    groupSet.forEach((groupID) => setByGroup.set(groupID, setIndex));
  });
  const affectedSets = new Set(
    [...changes.added, ...changes.removed].flatMap((groupID) => {
      const setIndex = setByGroup.get(groupID);
      return setIndex === undefined ? [] : [setIndex];
    }),
  );
  const target = decision.desired_group_ids.filter((groupID) => {
    const setIndex = setByGroup.get(groupID);
    return setIndex !== undefined && affectedSets.has(setIndex);
  });
  return { source: changes.removed, target, changes };
}

function plainPricingIssue(reason?: string | null) {
  const value = reason || "账号资料不足，无法计算目标分组";
  if (value.includes("未设置成本倍率")) return "未记录账号成本，暂时无法判断应进入哪个价格分组。";
  if (value.includes("成本倍率") && value.includes("无效"))
    return "账号成本必须大于 0，当前数值无法用于价格分组。";
  if (value.includes("平台缺失")) return "账号平台未记录，暂时无法匹配同平台的价格分组。";
  if (value.includes("分组数据无效")) return "当前分组资料不完整，本次不会修改。";
  if (value.includes("人工优先")) return "账号处于人工优先位，本次不会自动调整分组。";
  return value.replaceAll("成本倍率", "账号成本");
}

function pricingDecisionBasis(
  decision: PricingDecision,
  groups: PricingGroup[],
  config: PricingConfig,
) {
  if (decision.skipped) return [plainPricingIssue(decision.reason)];
  const cost = Number(decision.cost_multiplier);
  if (!Number.isFinite(cost) || cost <= 0) return ["账号成本必须大于 0，本次不会修改分组。"];
  const groupByID = new Map(groups.map((group) => [group.id, group]));
  const setByGroup = new Map<string, number>();
  config.exchange_group_sets.forEach((groupSet, setIndex) => {
    groupSet.forEach((groupID) => setByGroup.set(groupID, setIndex));
  });
  const activeSets = new Set<number>();
  decision.current_group_ids.forEach((groupID) => {
    const setIndex = setByGroup.get(groupID);
    if (setIndex !== undefined) activeSets.add(setIndex);
  });
  if (activeSets.size === 0) return ["当前分组不属于任何互换组，价格管理不会调整"];
  const rows: string[] = [];
  for (const setIndex of [...activeSets].sort((left, right) => left - right)) {
    for (const groupID of config.exchange_group_sets[setIndex] ?? []) {
      const group = groupByID.get(groupID);
      const name = group?.name ? `${group.name}（#${groupID}）` : `分组 #${groupID}`;
      if (!group?.available) {
        rows.push(`${name}：不可分配${group?.reason ? `，${group.reason}` : ""}`);
        continue;
      }
      if (group.platform !== decision.platform) {
        rows.push(
          `${name}：分组平台 ${group.platform || "未记录"} 与账号平台 ${decision.platform || "未记录"} 不一致`,
        );
        continue;
      }
      const sale = Number(group.rate_multiplier);
      if (!Number.isFinite(sale) || sale <= 0) {
        rows.push(`${name}：售价倍率无效`);
        continue;
      }
      const limit = sale * (1 - config.profit_margin);
      rows.push(
        `${name}：账号成本 ${decision.cost_multiplier} ${cost <= limit ? "≤" : ">"} 可接受成本 ${decimal(limit)}（售价 ${group.rate_multiplier} 扣除 ${percent(config.profit_margin)} 目标利润）`,
      );
    }
  }
  return rows;
}

function pricingDecisionReason(
  decision: PricingDecision,
  groups: PricingGroup[],
  config: PricingConfig,
) {
  if (decision.skipped) return plainPricingIssue(decision.reason);
  const basis = pricingDecisionBasis(decision, groups, config);
  if (!decision.changed) {
    return basis[0]?.startsWith("当前分组不属于")
      ? "当前分组不在价格自动调整范围内。"
      : `当前已经是能保证 ${percent(config.profit_margin)} 目标利润的最低售价分组。`;
  }
  const transition = pricingGroupTransition(decision, config);
  const groupByID = new Map(groups.map((group) => [group.id, group]));
  const targetGroups = transition.target.flatMap((groupID) => {
    const group = groupByID.get(groupID);
    const sale = Number(group?.rate_multiplier);
    return group && Number.isFinite(sale) && sale > 0 ? [{ group, sale }] : [];
  });
  if (targetGroups.length === 1) {
    const { group, sale } = targetGroups[0];
    return `账号成本 ${decision.cost_multiplier} 不高于 ${group.name} 可接受的 ${decimal(sale * (1 - config.profit_margin))}；这是仍能保证 ${percent(config.profit_margin)} 目标利润的最低售价分组。`;
  }
  if (targetGroups.length > 1) {
    return `已分别选择每个互换组中仍能保证 ${percent(config.profit_margin)} 目标利润的最低售价分组。`;
  }
  return `账号成本 ${decision.cost_multiplier} 已高于本互换组所有分组可接受的成本，继续使用会低于 ${percent(config.profit_margin)} 目标利润。`;
}

function pricingConfigIsValid(config: PricingConfig, groups: PricingGroup[]) {
  if (
    config.profit_margin < 0 ||
    config.profit_margin > 0.99 ||
    config.interval_seconds < 30 ||
    config.interval_seconds > 86400 ||
    config.write_concurrency < 1 ||
    config.write_concurrency > 16 ||
    (config.enabled && config.exchange_group_sets.length === 0)
  )
    return false;
  const byID = new Map(groups.map((group) => [group.id, group]));
  const seen = new Set<string>();
  for (const groupSet of config.exchange_group_sets) {
    if (groupSet.length < 2) return false;
    let platform = "";
    for (const groupID of groupSet) {
      const group = byID.get(groupID);
      if (seen.has(groupID) || !group?.available) return false;
      seen.add(groupID);
      if (!platform) platform = group.platform;
      if (platform !== group.platform) return false;
    }
  }
  return true;
}

export function pricingPreviewDecisions(
  decisions: PricingDecision[],
  groups: PricingGroup[],
  config: PricingConfig,
) {
  const setByGroup = new Map<string, number>();
  config.exchange_group_sets.forEach((groupSet, setIndex) => {
    groupSet.forEach((groupID) => setByGroup.set(groupID, setIndex));
  });
  const byID = new Map(groups.map((group) => [group.id, group]));
  return decisions.map((decision) => {
    const cost = Number(decision.cost_multiplier);
    if (decision.skipped || !Number.isFinite(cost) || cost <= 0) {
      return {
        ...decision,
        desired_group_ids: [...decision.current_group_ids],
        changed: false,
        skipped: true,
      };
    }
    const activeSets = new Set<number>();
    const currentBySet = new Map<number, string[]>();
    decision.current_group_ids.forEach((groupID) => {
      const setIndex = setByGroup.get(groupID);
      if (setIndex !== undefined) {
        activeSets.add(setIndex);
        currentBySet.set(setIndex, [...(currentBySet.get(setIndex) ?? []), groupID]);
      }
    });
    const desired = decision.current_group_ids.filter((groupID) => !setByGroup.has(groupID));
    const eligible: string[] = [];
    for (const setIndex of [...activeSets].sort((left, right) => left - right)) {
      const compatible: Array<{ group: PricingGroup; rate: number }> = [];
      for (const groupID of config.exchange_group_sets[setIndex] ?? []) {
        const group = byID.get(groupID);
        const rate = Number(group?.rate_multiplier);
        if (!group?.available || group.platform !== decision.platform || !Number.isFinite(rate))
          continue;
        compatible.push({ group, rate });
      }
      const chosen = compatible
        .filter(({ rate }) => cost <= rate * (1 - config.profit_margin))
        .sort(
          (left, right) => left.rate - right.rate || Number(left.group.id) - Number(right.group.id),
        )[0];
      const chosenGroup =
        chosen?.group ??
        (compatible.length === 0
          ? (currentBySet.get(setIndex) ?? [])
              .map((groupID) => byID.get(groupID))
              .filter((group): group is PricingGroup => Boolean(group))
              .sort((left, right) => Number(left.id) - Number(right.id))[0]
          : undefined);
      if (chosenGroup) {
        desired.push(chosenGroup.id);
        if (chosen) eligible.push(chosenGroup.name);
      }
    }
    const unique = [...new Set(desired)].sort((left, right) => Number(left) - Number(right));
    return {
      ...decision,
      desired_group_ids: unique,
      eligible_groups: eligible,
      changed: decision.current_group_ids.join(",") !== unique.join(","),
      skipped: false,
    };
  });
}

function PricingLoading() {
  return (
    <div className="space-y-4" data-testid="pricing-loading">
      <Skeleton className="h-48 w-full" />
      <Skeleton className="h-16 w-full" />
      <Skeleton className="h-72 w-full" />
    </div>
  );
}

function PricingCatalogTable(props: { groups: PricingGroup[]; decisions: PricingDecision[] }) {
  const [search, setSearch] = useState("");
  const [selectedGroup, setSelectedGroup] = useState<PricingGroup | null>(null);
  const filteredGroups = useMemo(() => {
    const query = search.trim().toLocaleLowerCase();
    if (!query) return props.groups;
    return props.groups.filter((group) =>
      [group.id, group.name, group.platform, group.status, group.reason].some((value) =>
        value?.toLocaleLowerCase().includes(query),
      ),
    );
  }, [props.groups, search]);
  const pagination = useClientPagination(filteredGroups);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <TableFilterToolbar>
        <SearchField
          value={search}
          onChange={(value) => {
            setSearch(value);
            pagination.setCurrentPage(1);
          }}
          placeholder="搜索分组、ID 或平台"
        />
      </TableFilterToolbar>
      <DataTablePanel className="flex-1" data-testid="pricing-catalog-table-frame">
        <Table className="min-w-[960px]" containerClassName="min-h-0 flex-1 overflow-auto">
          <TableHeader>
            <TableRow>
              <TableHead className="w-56">分组</TableHead>
              <TableHead className="w-40">平台</TableHead>
              <TableHead className="w-44">
                <AccountCostHeader range />
              </TableHead>
              <TableHead className="w-40">售价倍率</TableHead>
              <TableHead className="w-48">状态</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {pagination.visibleItems.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="text-muted-foreground h-24 text-center">
                  {search ? "没有匹配的分组" : "当前没有价格分组"}
                </TableCell>
              </TableRow>
            ) : (
              pagination.visibleItems.map((group) => (
                <TableRow key={group.id}>
                  <TableCell>
                    <span className="font-medium">{group.name}</span>
                    <span className="text-muted-foreground ml-2">#{group.id}</span>
                  </TableCell>
                  <TableCell>{group.platform || "-"}</TableCell>
                  <TableCell>
                    <GroupAccountCostCell
                      groupID={group.id}
                      decisions={props.decisions}
                      onOpen={() => setSelectedGroup(group)}
                    />
                  </TableCell>
                  <TableCell>{group.rate_multiplier ?? "-"}</TableCell>
                  <TableCell>
                    {group.available ? (
                      <Badge variant="outline">可用</Badge>
                    ) : (
                      <Badge variant="warning">{group.reason ?? "不可用"}</Badge>
                    )}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        {filteredGroups.length > 0 ? (
          <DataTablePagination
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            totalItems={filteredGroups.length}
            pageSize={pagination.pageSize}
            pageSizes={[10, 20, 50, 100]}
            onPageChange={pagination.setCurrentPage}
            onPageSizeChange={pagination.setPageSize}
          />
        ) : null}
      </DataTablePanel>
      <Dialog
        open={selectedGroup !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedGroup(null);
        }}
      >
        <DialogContent width="wide" height="large" className="grid-rows-[auto_minmax(0,1fr)]">
          <DialogHeader>
            <DialogTitle>账号成本档位明细</DialogTitle>
            {selectedGroup ? (
              <DialogDescription>
                {selectedGroup.name}（#{selectedGroup.id}） ·{" "}
                {groupAccountCosts(selectedGroup.id, props.decisions).accounts} 个账号 ·{" "}
                {groupAccountCosts(selectedGroup.id, props.decisions).values.length} 个成本档位
              </DialogDescription>
            ) : null}
          </DialogHeader>
          <DialogBody className="overflow-hidden pr-0">
            {selectedGroup ? (
              <GroupAccountCostDetails
                key={selectedGroup.id}
                groupID={selectedGroup.id}
                decisions={props.decisions}
              />
            ) : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export function PricingPreviewTable(props: {
  decisions: PricingDecision[];
  groups: PricingGroup[];
  config: PricingConfig;
}) {
  type PreviewFilter = "changed" | "unchanged" | "skipped" | "all";
  const [filter, setFilter] = useState<PreviewFilter>("changed");
  const [expandedAccountID, setExpandedAccountID] = useState<string | null>(null);
  const counts = useMemo(
    () => ({
      changed: props.decisions.filter((decision) => decision.changed && !decision.skipped).length,
      unchanged: props.decisions.filter((decision) => !decision.changed && !decision.skipped)
        .length,
      skipped: props.decisions.filter((decision) => decision.skipped).length,
      all: props.decisions.length,
    }),
    [props.decisions],
  );
  const filteredDecisions = useMemo(
    () =>
      props.decisions.filter((decision) => {
        if (filter === "changed") return decision.changed && !decision.skipped;
        if (filter === "unchanged") return !decision.changed && !decision.skipped;
        if (filter === "skipped") return decision.skipped;
        return true;
      }),
    [filter, props.decisions],
  );
  const pagination = useClientPagination(filteredDecisions, 10);
  const groupNames = useMemo(
    () => new Map(props.groups.map((group) => [group.id, group.name])),
    [props.groups],
  );

  return (
    <div
      className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-3"
      data-testid="pricing-preview-table"
    >
      <TableFilterToolbar aria-label="明细状态筛选">
        <SegmentedControl>
          {(
            [
              ["changed", "将调整"],
              ["unchanged", "无需调整"],
              ["skipped", "无法判定"],
              ["all", "全部"],
            ] as const
          ).map(([value, label]) => (
            <SegmentedControlItem
              key={value}
              type="button"
              selected={filter === value}
              onClick={() => {
                setFilter(value);
                pagination.setCurrentPage(1);
                setExpandedAccountID(null);
              }}
            >
              {label}
              <span className="text-muted-foreground tabular-nums">{counts[value]}</span>
            </SegmentedControlItem>
          ))}
        </SegmentedControl>
      </TableFilterToolbar>
      <DataTablePanel>
        <Table
          className="min-w-[68rem] table-fixed"
          containerClassName="min-h-0 flex-1 overflow-auto"
          overflowTooltip={false}
        >
          <TableHeader>
            <TableRow>
              <TableHead className="w-52">账号</TableHead>
              <TableHead className="w-28">
                <AccountCostHeader />
              </TableHead>
              <TableHead className="w-80">分组调整</TableHead>
              <TableHead>调整原因</TableHead>
              <TableHead className="w-24">状态</TableHead>
              <TableHead className="w-28">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredDecisions.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-muted-foreground h-24 text-center">
                  当前筛选条件下没有账号
                </TableCell>
              </TableRow>
            ) : (
              pagination.visibleItems.map((decision) => {
                const transition = pricingGroupTransition(decision, props.config);
                const basis = pricingDecisionBasis(decision, props.groups, props.config);
                const currentGroups = groupIDsLabel(decision.current_group_ids, groupNames);
                const reason = pricingDecisionReason(decision, props.groups, props.config);
                const expanded = expandedAccountID === decision.account_id;
                return (
                  <Fragment key={decision.account_id}>
                    <TableRow>
                      <TableCell className="align-top whitespace-normal">
                        <div className="grid gap-0.5">
                          <span className="break-words font-medium">
                            {decision.account_name || `账号 ${decision.account_id}`}
                          </span>
                          <span className="text-muted-foreground text-xs">
                            #{decision.account_id} · {decision.platform || "平台未记录"}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="align-top font-medium tabular-nums">
                        {decision.cost_multiplier ?? "-"}
                      </TableCell>
                      <TableCell className="align-top whitespace-normal">
                        {decision.changed && !decision.skipped ? (
                          <div className="flex min-w-0 items-center gap-2 text-xs leading-5">
                            <span className="text-destructive min-w-0 break-words font-medium">
                              {groupIDsLabel(transition.source, groupNames)}
                            </span>
                            <ArrowRight
                              className="text-muted-foreground size-4 shrink-0"
                              aria-hidden="true"
                            />
                            <span className="text-success min-w-0 break-words font-medium">
                              {groupIDsLabel(transition.target, groupNames)}
                            </span>
                          </div>
                        ) : (
                          <span className="text-muted-foreground break-words text-xs">
                            {currentGroups}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-muted-foreground align-top whitespace-normal">
                        <span className="break-words text-xs leading-5">{reason}</span>
                      </TableCell>
                      <TableCell className="align-top">
                        {decision.skipped ? (
                          <Badge variant="warning">无法判定</Badge>
                        ) : decision.changed ? (
                          <Badge variant="secondary">将调整</Badge>
                        ) : (
                          <Badge variant="outline">无需调整</Badge>
                        )}
                      </TableCell>
                      <TableCell className="align-top">
                        <Button
                          type="button"
                          size="xs"
                          variant="ghost"
                          aria-expanded={expanded}
                          aria-controls={`pricing-basis-${decision.account_id}`}
                          onClick={() =>
                            setExpandedAccountID(expanded ? null : decision.account_id)
                          }
                        >
                          <ChevronDown
                            className={cn("transition-transform", expanded && "rotate-180")}
                            aria-hidden="true"
                          />
                          计算明细
                        </Button>
                      </TableCell>
                    </TableRow>
                    {expanded ? (
                      <TableRow id={`pricing-basis-${decision.account_id}`}>
                        <TableCell colSpan={6} className="bg-muted/20 whitespace-normal px-4 py-3">
                          <div className="grid gap-1.5">
                            <span className="font-medium">计算明细</span>
                            {basis.map((line) => (
                              <span
                                className="text-muted-foreground break-words text-xs"
                                key={line}
                              >
                                {line}
                              </span>
                            ))}
                          </div>
                        </TableCell>
                      </TableRow>
                    ) : null}
                  </Fragment>
                );
              })
            )}
          </TableBody>
        </Table>
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={filteredDecisions.length}
          pageSize={pagination.pageSize}
          pageSizes={[10, 20, 50, 100]}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      </DataTablePanel>
    </div>
  );
}

type ExchangeGroupSetEditorProps = {
  setIndex: number;
  groupSet: string[];
  groups: PricingGroup[];
  exchangeSetByGroup: Map<string, number>;
  onToggle: (setIndex: number, groupID: string, checked: boolean) => void;
  onRemove: (setIndex: number) => void;
};

function ExchangeGroupSetEditor(props: ExchangeGroupSetEditorProps) {
  const [expanded, setExpanded] = useState(true);
  const setNumber = props.setIndex + 1;
  const contentID = `exchange-set-content-${setNumber}`;
  const selectedPlatforms = new Set(
    props.groups
      .filter((group) => props.groupSet.includes(group.id))
      .map((group) => group.platform || "未标注平台"),
  );
  const selectedPlatform = selectedPlatforms.size === 1 ? [...selectedPlatforms][0] : undefined;
  const platformMismatch = selectedPlatforms.size > 1;
  const visibleGroups = selectedPlatform
    ? props.groups.filter(
        (group) =>
          props.groupSet.includes(group.id) ||
          (group.platform || "未标注平台") === selectedPlatform,
      )
    : props.groups;
  const sections = new Map<string, PricingGroup[]>();
  for (const group of visibleGroups) {
    const platform = group.platform || "未标注平台";
    const section = sections.get(platform) ?? [];
    section.push(group);
    sections.set(platform, section);
  }
  const complete = props.groupSet.length >= 2 && !platformMismatch;
  let statusLabel = `已选 ${props.groupSet.length} / 至少 2 个`;
  if (platformMismatch) {
    statusLabel = "平台混用";
  } else if (complete) {
    statusLabel = `${props.groupSet.length} 个分组`;
  }

  return (
    <section
      data-testid={`exchange-set-${setNumber}`}
      aria-labelledby={`exchange-set-title-${setNumber}`}
    >
      <div className="bg-muted/20 flex min-h-10 items-center justify-between gap-3 px-3 py-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span id={`exchange-set-title-${setNumber}`} className="font-medium">
            互换组 {setNumber}
          </span>
          <Badge variant={complete ? "outline" : "warning"}>{statusLabel}</Badge>
          {selectedPlatform ? <Badge variant="secondary">{selectedPlatform}</Badge> : null}
          <span className="text-muted-foreground text-xs tabular-nums">
            {visibleGroups.filter((group) => group.available).length} 个可用
          </span>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`${expanded ? "收起" : "展开"}互换组 ${setNumber}`}
                  aria-expanded={expanded}
                  aria-controls={contentID}
                  onClick={() => setExpanded((value) => !value)}
                />
              }
            >
              <ChevronDown
                className={cn("transition-transform", expanded && "rotate-180")}
                aria-hidden="true"
              />
            </TooltipTrigger>
            <TooltipContent>{expanded ? "收起互换组" : "展开互换组"}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`删除互换组 ${setNumber}`}
                  onClick={() => props.onRemove(props.setIndex)}
                />
              }
            >
              <Trash2 />
            </TooltipTrigger>
            <TooltipContent>删除互换组</TooltipContent>
          </Tooltip>
        </div>
      </div>

      <div
        id={contentID}
        className="space-y-3 px-3 py-2.5"
        data-testid={`exchange-set-options-${setNumber}`}
        role="group"
        aria-label={`互换组 ${setNumber} 可选分组`}
        hidden={!expanded}
      >
        {[...sections.entries()].map(([platform, groups]) => (
          <div key={platform} data-platform-section={platform} className="space-y-1.5">
            {sections.size > 1 || !selectedPlatform ? (
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground text-xs font-medium">{platform}</span>
                <span className="bg-border h-px min-w-4 flex-1" aria-hidden="true" />
                <span className="text-muted-foreground text-xs tabular-nums">
                  {groups.filter((group) => group.available).length} 个可用
                </span>
              </div>
            ) : null}
            <div className="grid gap-1.5 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
              {groups.map((group) => {
                const assignedSet = props.exchangeSetByGroup.get(group.id);
                const selected = assignedSet === props.setIndex;
                const assignedElsewhere = assignedSet !== undefined && !selected;
                const groupPlatform = group.platform || "未标注平台";
                const wrongPlatform = Boolean(
                  selectedPlatform && groupPlatform !== selectedPlatform,
                );
                const disabled =
                  !selected && (assignedElsewhere || !group.available || wrongPlatform);
                let detail = group.rate_multiplier
                  ? `售价 ${group.rate_multiplier}`
                  : `#${group.id}`;
                if (!group.available) detail = "不可用";
                if (assignedElsewhere) detail = `互换组 ${assignedSet + 1}`;
                if (wrongPlatform) detail = "其他平台";
                return (
                  <label
                    key={`${props.setIndex}:${group.id}`}
                    data-slot="exchange-group-option"
                    data-selected={selected ? "true" : "false"}
                    className={cn(
                      "flex min-h-9 min-w-0 items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm transition-colors",
                      selected ? "border-primary/50 bg-primary/5" : "hover:bg-muted/40",
                      disabled && "bg-muted/20 text-muted-foreground",
                    )}
                  >
                    <Checkbox
                      checked={selected}
                      disabled={disabled}
                      onCheckedChange={(checked) =>
                        props.onToggle(props.setIndex, group.id, checked)
                      }
                      aria-label={`互换组 ${props.setIndex + 1} 分组 ${group.name}`}
                    />
                    <span className="min-w-0 flex-1 truncate font-medium">{group.name}</span>
                    <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
                      {detail}
                    </span>
                  </label>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function PricingWorkspace(props: { page: "catalog" | "config" }) {
  const queryClient = useQueryClient();
  const snapshot = useQuery({ queryKey: ["pricing"], queryFn: api.pricing });
  const [draft, setDraft] = useState<PricingConfig | null>(null);
  const [taskID, setTaskID] = useState<string | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [backupDialog, setBackupDialog] = useState<"create" | "restore" | null>(null);
  const [backupName, setBackupName] = useState("");
  const [selectedBackupID, setSelectedBackupID] = useState("");
  const backups = useQuery({
    queryKey: ["pricing-backups"],
    queryFn: api.pricingBackups,
    enabled: props.page === "catalog",
  });
  const task = useQuery({
    queryKey: ["task", taskID],
    queryFn: () => api.task(taskID!),
    enabled: Boolean(taskID),
    refetchInterval: taskPollInterval,
  });

  useEffect(() => {
    if (snapshot.data) setDraft(snapshot.data.config);
  }, [snapshot.data]);

  useEffect(() => {
    if (!taskID || !taskStopsPolling(task.data)) return;
    if (task.data?.status === "succeeded") {
      toast.success(task.data.message || "价格分组调整完成");
      void queryClient.invalidateQueries({ queryKey: ["pricing"] });
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      void queryClient.invalidateQueries({ queryKey: ["groups"] });
    } else if (task.data) {
      toast.error(task.data.message || "价格分组调整失败");
    }
    setTaskID(null);
  }, [queryClient, task.data, taskID]);

  const save = useMutation({
    mutationFn: api.updatePricingConfig,
    onSuccess: (saved) => {
      queryClient.setQueryData(["pricing"], saved);
      setDraft(saved.config);
      toast.success(
        saved.config.enabled ? "价格配置已保存并开启" : "价格配置已保存，自动调整保持关闭",
      );
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "价格配置保存失败"),
  });
  const apply = useMutation({
    mutationFn: api.applyPricing,
    onSuccess: (queued) => {
      setTaskID(queued.id);
      queryClient.setQueryData(["task", queued.id], queued);
      toast.success("价格分组调整已开始");
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "价格分组调整启动失败"),
  });
  const createBackup = useMutation({
    mutationFn: api.createPricingBackup,
    onSuccess: (backup) => {
      setBackupName("");
      setBackupDialog(null);
      void queryClient.invalidateQueries({ queryKey: ["pricing-backups"] });
      toast.success(`备份“${backup.name}”已创建`);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "价格分组备份失败"),
  });
  const restoreBackup = useMutation({
    mutationFn: api.restorePricingBackup,
    onSuccess: (queued) => {
      setTaskID(queued.id);
      queryClient.setQueryData(["task", queued.id], queued);
      setBackupDialog(null);
      toast.success("价格分组备份还原已开始");
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "价格分组备份还原启动失败"),
  });
  const current = draft ?? snapshot.data?.config ?? null;
  const exchangeSetByGroup = useMemo(() => {
    const result = new Map<string, number>();
    current?.exchange_group_sets.forEach((groupSet, setIndex) => {
      groupSet.forEach((groupID) => result.set(groupID, setIndex));
    });
    return result;
  }, [current]);
  const previewDecisions = useMemo(
    () =>
      current && snapshot.data
        ? pricingPreviewDecisions(snapshot.data.decisions, snapshot.data.groups, current)
        : [],
    [current, snapshot.data],
  );
  const running = Boolean(taskID) && !taskStopsPolling(task.data);
  const valid = Boolean(
    current && snapshot.data && pricingConfigIsValid(current, snapshot.data.groups),
  );

  function toggleExchangeGroup(setIndex: number, groupID: string, checked: boolean) {
    if (!current) return;
    const sets = current.exchange_group_sets.map((groupSet) => [...groupSet]);
    sets[setIndex] = checked
      ? [...new Set([...sets[setIndex], groupID])].sort(
          (left, right) => Number(left) - Number(right),
        )
      : sets[setIndex].filter((id) => id !== groupID);
    setDraft({ ...current, exchange_group_sets: sets });
  }

  function addExchangeGroupSet() {
    if (!current) return;
    setDraft({ ...current, exchange_group_sets: [...current.exchange_group_sets, []] });
  }

  function removeExchangeGroupSet(setIndex: number) {
    if (!current) return;
    setDraft({
      ...current,
      exchange_group_sets: current.exchange_group_sets.filter((_, index) => index !== setIndex),
    });
  }

  return (
    <PageLayout fixedContent={props.page === "catalog"}>
      <PageHeading
        eyebrow={props.page === "catalog" ? "OPERATIONS / PRICING" : "POLICY / PRICING"}
        title={props.page === "catalog" ? "价格管理" : "价格配置"}
        description={
          props.page === "catalog"
            ? "查看各业务分组当前使用的售价倍率。"
            : "配置盈利目标、任务执行参数和账号互换范围。"
        }
        action={
          <PageActions data-testid="pricing-page-actions">
            <Button
              variant="outline"
              onClick={() => void snapshot.refetch()}
              disabled={snapshot.isFetching}
            >
              <RefreshCw /> 刷新
            </Button>
            {props.page === "catalog" ? (
              <>
                <Button
                  variant="outline"
                  onClick={() => {
                    setBackupName("");
                    setBackupDialog("create");
                  }}
                  disabled={!snapshot.data}
                >
                  <DatabaseBackup /> 创建备份
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    setSelectedBackupID(backups.data?.[0]?.id ?? "");
                    setBackupDialog("restore");
                  }}
                  disabled={!backups.data?.length || running}
                >
                  <RotateCcw /> 从备份还原
                </Button>
                <Button
                  variant="outline"
                  onClick={() => setPreviewOpen(true)}
                  disabled={!snapshot.data}
                >
                  <Eye /> 查看账号调整明细
                </Button>
              </>
            ) : null}
            {props.page === "config" ? (
              <>
                <Button
                  variant="outline"
                  onClick={() => current && save.mutate(current)}
                  disabled={!valid || save.isPending}
                >
                  <Save /> {save.isPending ? "保存中" : "保存配置"}
                </Button>
                <Button
                  onClick={() => apply.mutate()}
                  disabled={!current?.enabled || running || apply.isPending}
                >
                  <Play /> {running ? "执行中" : "立即调整"}
                </Button>
              </>
            ) : null}
          </PageActions>
        }
      />

      {snapshot.error ? (
        <QueryErrorToast error={snapshot.error} fallback="价格数据读取失败" />
      ) : null}
      {snapshot.isLoading || !snapshot.data ? (
        <PricingLoading />
      ) : props.page === "catalog" ? (
        <div className="flex h-full min-h-0 flex-col" data-testid="pricing-page">
          <PricingCatalogTable groups={snapshot.data.groups} decisions={snapshot.data.decisions} />
        </div>
      ) : current ? (
        <div className="space-y-3" data-testid="pricing-config-page">
          <Card size="sm" data-testid="pricing-settings-panel">
            <CardHeader className="grid-cols-1 sm:grid-cols-[minmax(0,1fr)_auto]">
              <div className="flex min-w-0 items-center gap-3">
                <span className="bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md">
                  <CircleDollarSign className="size-4" aria-hidden="true" />
                </span>
                <div className="min-w-0">
                  <CardTitle>自动价格分组</CardTitle>
                  <CardDescription>
                    设置盈利目标和自动执行参数，执行参数不参与售价计算；每个互换组只选择一个满足利润且售价最低的分组。
                  </CardDescription>
                </div>
              </div>
              <div className="flex items-center gap-2 justify-self-start sm:justify-self-end">
                <Badge variant={current.enabled ? "default" : "secondary"}>
                  {current.enabled ? "已开启" : "默认关闭"}
                </Badge>
                <Switch
                  checked={current.enabled}
                  onCheckedChange={(enabled) => setDraft({ ...current, enabled })}
                  aria-label="启用动态价格分组"
                />
              </div>
            </CardHeader>
            <CardContent
              className="grid divide-y p-0 lg:grid-cols-3 lg:divide-x lg:divide-y-0"
              data-testid="pricing-settings-grid"
            >
              <label
                className="grid gap-3 px-3 py-2.5 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center lg:grid-cols-1 lg:items-start 2xl:grid-cols-[minmax(0,1fr)_8.5rem] 2xl:items-center"
                data-testid="pricing-goal-settings"
              >
                <span className="min-w-0">
                  <span className="block text-sm font-medium">目标盈利比例</span>
                  <span className="text-muted-foreground block text-xs">允许范围 0% - 99%</span>
                </span>
                <span className="relative block">
                  <Input
                    className="pr-8 tabular-nums"
                    type="number"
                    min={0}
                    max={99}
                    step="0.1"
                    value={Number((current.profit_margin * 100).toFixed(4))}
                    onChange={(event) =>
                      setDraft({ ...current, profit_margin: Number(event.target.value) / 100 })
                    }
                  />
                  <span className="text-muted-foreground pointer-events-none absolute inset-y-0 right-2.5 flex items-center text-xs">
                    %
                  </span>
                </span>
              </label>
              <label
                className="grid gap-3 px-3 py-2.5 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center lg:grid-cols-1 lg:items-start 2xl:grid-cols-[minmax(0,1fr)_8.5rem] 2xl:items-center"
                data-testid="pricing-execution-settings"
              >
                <span className="min-w-0">
                  <span className="block text-sm font-medium">动态调整间隔</span>
                  <span className="text-muted-foreground block text-xs">30 秒 - 24 小时</span>
                </span>
                <span className="relative block">
                  <Input
                    className="pr-9 tabular-nums"
                    type="number"
                    min={30}
                    max={86400}
                    value={current.interval_seconds}
                    onChange={(event) =>
                      setDraft({ ...current, interval_seconds: Number(event.target.value) })
                    }
                  />
                  <span className="text-muted-foreground pointer-events-none absolute inset-y-0 right-2.5 flex items-center text-xs">
                    秒
                  </span>
                </span>
              </label>
              <label className="grid gap-3 px-3 py-2.5 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center lg:grid-cols-1 lg:items-start 2xl:grid-cols-[minmax(0,1fr)_8.5rem] 2xl:items-center">
                <span className="min-w-0">
                  <span className="block text-sm font-medium">写入并发</span>
                  <span className="text-muted-foreground block text-xs">允许范围 1 - 16</span>
                </span>
                <Input
                  className="tabular-nums"
                  type="number"
                  min={1}
                  max={16}
                  value={current.write_concurrency}
                  onChange={(event) =>
                    setDraft({ ...current, write_concurrency: Number(event.target.value) })
                  }
                />
              </label>
            </CardContent>
          </Card>

          <Card size="sm">
            <CardHeader className="flex items-start justify-between gap-3 sm:flex-row sm:items-center">
              <div className="flex min-w-0 items-center gap-3">
                <span className="bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md">
                  <ArrowLeftRight className="size-4" aria-hidden="true" />
                </span>
                <div className="min-w-0">
                  <CardTitle className="flex items-center gap-2">
                    账号互换范围
                    <Badge variant="secondary">{current.exchange_group_sets.length} 组</Badge>
                  </CardTitle>
                  <CardDescription>
                    同一互换组仅允许选择相同平台的分组，各组之间完全隔离。
                  </CardDescription>
                </div>
              </div>
              <Button size="sm" variant="outline" onClick={addExchangeGroupSet}>
                <Plus /> 添加互换组
              </Button>
            </CardHeader>
            <CardContent className="divide-y p-0">
              {current.exchange_group_sets.length === 0 ? (
                <div
                  className="flex min-h-28 flex-wrap items-center justify-center gap-x-4 gap-y-3 px-4 py-5"
                  data-testid="exchange-groups-empty"
                >
                  <span className="bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-md">
                    <ArrowLeftRight className="size-4" aria-hidden="true" />
                  </span>
                  <div className="min-w-0 text-center sm:text-left">
                    <p className="font-medium">还没有互换组</p>
                    <p className="text-muted-foreground text-xs">
                      自动调价暂时不会调整账号所属分组。
                    </p>
                  </div>
                  <Button size="sm" onClick={addExchangeGroupSet}>
                    <Plus /> 创建第一个互换组
                  </Button>
                </div>
              ) : (
                current.exchange_group_sets.map((groupSet, setIndex) => (
                  <ExchangeGroupSetEditor
                    key={`exchange-set-${setIndex}`}
                    setIndex={setIndex}
                    groupSet={groupSet}
                    groups={snapshot.data.groups}
                    exchangeSetByGroup={exchangeSetByGroup}
                    onToggle={toggleExchangeGroup}
                    onRemove={removeExchangeGroupSet}
                  />
                ))
              )}
            </CardContent>
          </Card>
        </div>
      ) : (
        <PricingLoading />
      )}
      {current && snapshot.data ? (
        <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
          <DialogContent
            width="table"
            height="tall"
            className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
          >
            <DialogHeader>
              <DialogTitle>账号分组调整明细</DialogTitle>
              <DialogDescription>
                按目标盈利比例 {percent(current.profit_margin)}{" "}
                计算。默认只显示需要调整的账号，完整公式可在每行的计算明细中查看。
              </DialogDescription>
            </DialogHeader>
            <DialogBody className="overflow-hidden pr-0">
              <PricingPreviewTable
                decisions={previewDecisions}
                groups={snapshot.data.groups}
                config={current}
              />
            </DialogBody>
          </DialogContent>
        </Dialog>
      ) : null}
      <Dialog
        open={backupDialog === "create"}
        onOpenChange={(open) => {
          if (!open) setBackupDialog(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>创建价格分组备份</DialogTitle>
            <DialogDescription>
              保存当前全部账号与本地分组的对应关系，之后可按稳定 ID 还原。
            </DialogDescription>
          </DialogHeader>
          <DialogBody>
            <label className="grid gap-2 text-sm font-medium">
              备份名称
              <Input
                value={backupName}
                maxLength={80}
                placeholder="例如：调整前基线"
                onChange={(event) => setBackupName(event.target.value)}
              />
            </label>
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => setBackupDialog(null)}>
              取消
            </Button>
            <Button
              disabled={!backupName.trim() || createBackup.isPending}
              onClick={() => createBackup.mutate(backupName.trim())}
            >
              <DatabaseBackup /> {createBackup.isPending ? "备份中" : "创建备份"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={backupDialog === "restore"}
        onOpenChange={(open) => {
          if (!open) setBackupDialog(null);
        }}
      >
        <DialogContent height="medium" className="grid grid-rows-[auto_minmax(0,1fr)_auto]">
          <DialogHeader>
            <DialogTitle>从备份还原分组</DialogTitle>
            <DialogDescription>
              将批量改写管理平台账号分组。已不存在的账号或分组会跳过，人工优先账号不会被修改。
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="grid content-start gap-2 pr-1">
            {backups.isError ? (
              <p className="text-destructive text-sm">价格分组备份读取失败</p>
            ) : null}
            {backups.data?.map((backup) => {
              const selected = backup.id === selectedBackupID;
              return (
                <button
                  key={backup.id}
                  type="button"
                  data-selected={selected}
                  aria-pressed={selected}
                  className="data-[selected=true]:border-primary data-[selected=true]:bg-primary/5 hover:bg-muted/40 flex items-center justify-between gap-3 rounded-lg border px-3 py-2.5 text-left"
                  onClick={() => setSelectedBackupID(backup.id)}
                >
                  <span className="min-w-0">
                    <span className="block truncate font-medium">{backup.name}</span>
                    <span className="text-muted-foreground block text-xs">
                      {new Date(backup.created_at).toLocaleString("zh-CN", { hour12: false })}
                    </span>
                  </span>
                  <Badge variant="secondary">{backup.account_count} 个账号</Badge>
                </button>
              );
            })}
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => setBackupDialog(null)}>
              取消
            </Button>
            <Button
              disabled={!selectedBackupID || restoreBackup.isPending || running}
              onClick={() => restoreBackup.mutate(selectedBackupID)}
            >
              <RotateCcw /> {restoreBackup.isPending ? "正在提交" : "确认还原"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PageLayout>
  );
}

export function PricingPage() {
  return <PricingWorkspace page="catalog" />;
}

export function PricingConfigPage() {
  return <PricingWorkspace page="config" />;
}
