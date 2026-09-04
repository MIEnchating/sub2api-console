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
  History,
  Play,
  Plus,
  RotateCcw,
  Save,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";

import {
  api,
  type PricingBackup,
  type PricingAccountChange,
  type PricingChangeGroup,
  type PricingChangeRecord,
  type PricingConfig,
  type PricingDecision,
  type PricingGroup,
} from "@/api";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { RefreshButton } from "@/components/refresh-button";
import { QueryErrorToast } from "@/components/query-error-toast";
import { DataTablePagination } from "@/components/data-table/pagination";
import { SearchField } from "@/components/data-table/search-field";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { FieldLabel } from "@/components/field-help-tooltip";
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
import { terminalRefreshKeys } from "@/lib/task-refresh";
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

function pricingConfigWithRuleNames(config: PricingConfig): PricingConfig {
  const currentNames = config.exchange_group_set_names ?? [];
  if (
    currentNames.length === config.exchange_group_sets.length &&
    currentNames.every((name) => name.trim() !== "")
  )
    return config;
  return {
    ...config,
    exchange_group_set_names: config.exchange_group_sets.map(
      (_, setIndex) => currentNames[setIndex]?.trim() || `互换组 ${setIndex + 1}`,
    ),
  };
}

export function pricingRuleNameForInput(names: string[], setIndex: number) {
  return names[setIndex] ?? `互换组 ${setIndex + 1}`;
}

function pricingConfigsEqual(left: PricingConfig, right: PricingConfig): boolean {
  return (
    left.enabled === right.enabled &&
    left.profit_margin === right.profit_margin &&
    left.interval_seconds === right.interval_seconds &&
    left.write_concurrency === right.write_concurrency &&
    left.exchange_group_sets.length === right.exchange_group_sets.length &&
    left.exchange_group_set_names.length === right.exchange_group_set_names.length &&
    left.exchange_group_set_names.every(
      (name, setIndex) => name === right.exchange_group_set_names[setIndex],
    ) &&
    left.exchange_group_sets.every((groupSet, setIndex) => {
      const other = right.exchange_group_sets[setIndex];
      return (
        groupSet.length === other.length &&
        groupSet.every((groupID, index) => groupID === other[index])
      );
    })
  );
}

export async function applyPricingWithDraft<Result>(
  draft: PricingConfig,
  persisted: PricingConfig,
  save: (config: PricingConfig) => Promise<unknown>,
  apply: () => Promise<Result>,
): Promise<Result> {
  if (!pricingConfigsEqual(draft, persisted)) await save(draft);
  return apply();
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
        {summary.accounts} 个账号
        {summary.values.length > 1 ? ` · ${summary.values.length} 档` : ""}
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
      <DataTablePanel className="flex-1">
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
      const limit = acceptableAccountCost(sale, config.profit_margin);
      rows.push(
        `${name}：账号成本 ${decision.cost_multiplier} ${cost <= limit ? "≤" : ">"} 可接受成本 ${decimal(limit)}（售价 ${group.rate_multiplier} ÷（1 + ${percent(config.profit_margin)}），目标盈利比例按利润 ÷ 账号成本计算）`,
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
  if (decision.reason?.includes("没有满足盈利比例")) {
    return `账号成本 ${decision.cost_multiplier} 已高于本互换组所有分组可接受的成本；本次保留当前分组。请降低目标盈利比例或提高候选分组售价。`;
  }
  const basis = pricingDecisionBasis(decision, groups, config);
  if (!decision.changed) {
    return basis[0]?.startsWith("当前分组不属于")
      ? "当前分组不在价格自动调整范围内。"
      : `当前已经是能保证 ${percent(config.profit_margin)} 目标成本利润率的最低售价分组。`;
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
    return `账号成本 ${decision.cost_multiplier} 不高于 ${group.name} 可接受的 ${decimal(acceptableAccountCost(sale, config.profit_margin))}；这是仍能保证 ${percent(config.profit_margin)} 目标成本利润率的最低售价分组。`;
  }
  if (targetGroups.length > 1) {
    return `已分别选择每个互换组中仍能保证 ${percent(config.profit_margin)} 目标成本利润率的最低售价分组。`;
  }
  return `账号成本 ${decision.cost_multiplier} 已高于本互换组所有分组可接受的成本，继续使用会低于 ${percent(config.profit_margin)} 目标成本利润率。`;
}

type PricingConfigDraft = Omit<
  PricingConfig,
  "profit_margin" | "interval_seconds" | "write_concurrency"
> & {
  profit_margin: number | null;
  interval_seconds: number | null;
  write_concurrency: number | null;
};

function pricingConfigIsValid(config: PricingConfigDraft, groups: PricingGroup[]) {
  if (
    config.profit_margin === null ||
    config.interval_seconds === null ||
    config.write_concurrency === null ||
    !Number.isFinite(config.profit_margin) ||
    !Number.isFinite(config.interval_seconds) ||
    !Number.isFinite(config.write_concurrency) ||
    config.profit_margin < 0 ||
    config.profit_margin > 0.99 ||
    config.interval_seconds < 30 ||
    config.interval_seconds > 86400 ||
    config.write_concurrency < 1 ||
    config.write_concurrency > 16 ||
    config.exchange_group_set_names.length !== config.exchange_group_sets.length ||
    config.exchange_group_set_names.some(
      (name) => name.trim().length === 0 || [...name.trim()].length > 64,
    ) ||
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
    let reason: string | null = null;
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
        .filter(({ rate }) => cost <= acceptableAccountCost(rate, config.profit_margin))
        .sort(
          (left, right) => left.rate - right.rate || Number(left.group.id) - Number(right.group.id),
        )[0];
      let chosenGroup = chosen?.group;
      if (!chosenGroup) {
        chosenGroup = (currentBySet.get(setIndex) ?? [])
          .map((groupID) => byID.get(groupID))
          .filter((group): group is PricingGroup => Boolean(group))
          .sort((left, right) => Number(left.id) - Number(right.id))[0];
        if (compatible.length > 0) {
          reason = "没有满足盈利比例的可用分组，保留当前分组";
        }
      }
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
      reason,
    };
  });
}

function acceptableAccountCost(sale: number, profitMargin: number): number {
  return sale / (1 + profitMargin);
}

export function PricingBackupList(props: {
  backups: PricingBackup[];
  selectedBackupID: string;
  onSelect: (backupID: string) => void;
  onDelete: (backup: PricingBackup) => void;
}) {
  if (props.backups.length === 0) {
    return <p className="text-muted-foreground py-6 text-center text-sm">暂无可还原的备份</p>;
  }
  return props.backups.map((backup) => {
    const selected = backup.id === props.selectedBackupID;
    return (
      <div
        key={backup.id}
        data-selected={selected}
        className="data-[selected=true]:border-primary data-[selected=true]:bg-primary/5 hover:bg-muted/40 flex items-stretch rounded-lg border"
      >
        <button
          type="button"
          aria-label={`选择备份 ${backup.name}`}
          aria-pressed={selected}
          className="flex min-w-0 flex-1 items-center justify-between gap-3 px-3 py-2.5 text-left"
          onClick={() => props.onSelect(backup.id)}
        >
          <span className="min-w-0">
            <span className="block truncate font-medium">{backup.name}</span>
            <span className="text-muted-foreground block text-xs">
              {new Date(backup.created_at).toLocaleString("zh-CN", { hour12: false })}
            </span>
          </span>
          <Badge variant="secondary">{backup.account_count} 个账号</Badge>
        </button>
        <span className="flex shrink-0 items-center border-l px-2">
          <Tooltip>
            <TooltipTrigger render={<span className="inline-flex" />}>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                className="text-destructive"
                aria-label={`删除备份 ${backup.name}`}
                onClick={() => props.onDelete(backup)}
              >
                <Trash2 aria-hidden="true" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>删除备份</TooltipContent>
          </Tooltip>
        </span>
      </div>
    );
  });
}

function pricingChangeGroupsLabel(groups: PricingChangeGroup[]): string {
  if (groups.length === 0) return "未分组";
  return groups.map((group) => `${group.name}（#${group.id}）`).join("、");
}

function pricingChangeDate(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "时间未记录";
  return date.toLocaleString("zh-CN", { hour12: false });
}

export function PricingChangeList(props: { records: PricingChangeRecord[] }) {
  const rows = useMemo(() => {
    const nextRows: Array<{
      record: PricingChangeRecord;
      change: PricingAccountChange | null;
    }> = [];
    for (const record of props.records) {
      const changes = record.changes ?? [];
      if (changes.length > 0) {
        nextRows.push(...changes.map((change) => ({ record, change })));
        continue;
      }
      if ((record.account_count ?? 0) > 0) {
        nextRows.push({ record, change: null });
      }
    }
    return nextRows;
  }, [props.records]);
  const pagination = useClientPagination(rows);

  if (rows.length === 0) {
    return (
      <div
        className="text-muted-foreground flex h-full min-h-40 flex-col items-center justify-center gap-2 text-sm"
        data-testid="pricing-changes-empty"
      >
        <History className="size-5" aria-hidden="true" />
        <span>暂无账号分组变更</span>
        <span className="text-xs">这里只记录实际写入的分组变化；无变化的执行不会生成记录。</span>
      </div>
    );
  }
  return (
    <div className="grid h-full min-h-0 grid-rows-[minmax(0,1fr)]">
      <DataTablePanel className="flex-1" data-testid="pricing-change-list">
        <Table
          className="min-w-[68rem] table-fixed"
          containerClassName="min-h-0 flex-1 overflow-auto"
          overflowTooltip={false}
        >
          <TableHeader>
            <TableRow>
              <TableHead className="w-48">变更时间</TableHead>
              <TableHead className="w-48">账号</TableHead>
              <TableHead>调整前</TableHead>
              <TableHead className="w-8" aria-label="调整为" />
              <TableHead>调整后</TableHead>
              <TableHead className="w-32">操作人</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {pagination.visibleItems.map((row) => (
              <TableRow key={`${row.record.id}-${row.change?.account_id ?? "batch"}`}>
                <TableCell className="text-muted-foreground align-top whitespace-normal tabular-nums">
                  <span className="text-xs leading-5">
                    {pricingChangeDate(row.record.created_at)}
                  </span>
                </TableCell>
                {row.change ? (
                  <>
                    <TableCell className="align-top whitespace-normal">
                      <div className="grid gap-0.5">
                        <span className="break-words font-medium">{row.change.account_name}</span>
                        <span className="text-muted-foreground text-xs">
                          #{row.change.account_id}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="align-top whitespace-normal">
                      <span className="text-destructive break-words text-xs leading-5 font-medium">
                        {pricingChangeGroupsLabel(row.change.before)}
                      </span>
                    </TableCell>
                    <TableCell className="text-muted-foreground px-0 text-center align-top">
                      <ArrowRight className="mx-auto mt-0.5 size-4" aria-hidden="true" />
                    </TableCell>
                    <TableCell className="align-top whitespace-normal">
                      <span className="text-success break-words text-xs leading-5 font-medium">
                        {pricingChangeGroupsLabel(row.change.after)}
                      </span>
                    </TableCell>
                  </>
                ) : (
                  <>
                    <TableCell
                      className="align-top whitespace-normal"
                      data-slot="pricing-change-batch-summary"
                    >
                      <div className="grid gap-0.5">
                        <span className="break-words font-medium">
                          {row.record.account_count ?? 0} 个账号
                        </span>
                        <span className="text-muted-foreground text-xs">历史批次汇总</span>
                      </div>
                    </TableCell>
                    <TableCell
                      colSpan={3}
                      className="text-muted-foreground align-top whitespace-normal"
                    >
                      <span className="break-words text-xs leading-5">
                        旧记录未保存账号级明细
                        {(row.record.group_link_count ?? 0) > 0
                          ? ` · ${row.record.group_link_count} 条成员关系`
                          : null}
                      </span>
                    </TableCell>
                  </>
                )}
                <TableCell className="align-top whitespace-normal">
                  <span className="break-words text-xs leading-5">
                    {row.record.actor || "系统任务"}
                  </span>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <DataTablePagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          totalItems={rows.length}
          pageSize={pagination.pageSize}
          pageSizes={[10, 20, 30, 50, 100]}
          onPageChange={pagination.setCurrentPage}
          onPageSizeChange={pagination.setPageSize}
        />
      </DataTablePanel>
    </div>
  );
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
                {selectedGroup.name}（#{selectedGroup.id}）
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
  const pagination = useClientPagination(filteredDecisions);
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
            </SegmentedControlItem>
          ))}
        </SegmentedControl>
      </TableFilterToolbar>
      <DataTablePanel className="flex-1">
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
  name: string;
  groupSet: string[];
  groups: PricingGroup[];
  exchangeSetByGroup: Map<string, number>;
  exchangeSetNames: string[];
  onToggle: (setIndex: number, groupID: string, checked: boolean) => void;
  onNameChange: (setIndex: number, name: string) => void;
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
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <span
            id={`exchange-set-title-${setNumber}`}
            className="text-muted-foreground shrink-0 text-xs font-medium"
          >
            规则 {setNumber}
          </span>
          <Input
            className="h-8 min-w-36 max-w-64 flex-1 font-medium"
            value={props.name}
            maxLength={64}
            aria-label={`互换组 ${setNumber} 规则名称`}
            placeholder={`互换组 ${setNumber}`}
            onChange={(event) => props.onNameChange(props.setIndex, event.target.value)}
          />
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
                if (assignedElsewhere)
                  detail = props.exchangeSetNames[assignedSet] || `互换组 ${assignedSet + 1}`;
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
                    <Tooltip>
                      <TooltipTrigger
                        render={
                          <span className="text-muted-foreground max-w-28 shrink-0 truncate text-xs tabular-nums" />
                        }
                      >
                        {detail}
                      </TooltipTrigger>
                      <TooltipContent className="max-w-sm">{detail}</TooltipContent>
                    </Tooltip>
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
  const [draft, setDraft] = useState<PricingConfigDraft | null>(null);
  const [taskID, setTaskID] = useState<string | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [changesOpen, setChangesOpen] = useState(false);
  const [backupDialog, setBackupDialog] = useState<"create" | "restore" | null>(null);
  const [backupName, setBackupName] = useState("");
  const [selectedBackupID, setSelectedBackupID] = useState("");
  const [deleteBackupTarget, setDeleteBackupTarget] = useState<PricingBackup | null>(null);
  const backups = useQuery({
    queryKey: ["pricing-backups"],
    queryFn: api.pricingBackups,
    enabled: props.page === "catalog",
  });
  const changes = useQuery({
    queryKey: ["pricing-changes"],
    queryFn: api.pricingChanges,
    enabled: props.page === "catalog" && changesOpen,
  });
  const task = useQuery({
    queryKey: ["task", taskID],
    queryFn: () => api.task(taskID!),
    enabled: Boolean(taskID),
    refetchInterval: taskPollInterval,
  });

  useEffect(() => {
    if (snapshot.data) setDraft(pricingConfigWithRuleNames(snapshot.data.config));
  }, [snapshot.data]);

  useEffect(() => {
    if (!taskID || !taskStopsPolling(task.data)) return;
    for (const queryKey of terminalRefreshKeys("pricing", task.data)) {
      void queryClient.invalidateQueries({ queryKey });
    }
    if (task.data?.status === "succeeded") {
      toast.success(task.data.message || "价格分组调整完成");
    } else if (task.data) {
      toast.error(task.data.message || "价格分组调整失败");
    }
    setTaskID(null);
  }, [queryClient, task.data, taskID]);

  const save = useMutation({
    mutationFn: api.updatePricingConfig,
    onSuccess: (saved) => {
      queryClient.setQueryData(["pricing"], saved);
      setDraft(pricingConfigWithRuleNames(saved.config));
      toast.success(
        saved.config.enabled ? "价格配置已保存并开启" : "价格配置已保存，自动调整保持关闭",
      );
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "价格配置保存失败"),
  });
  const apply = useMutation({
    mutationFn: () => {
      if (!current || !snapshot.data || !pricingConfigIsValid(current, snapshot.data.groups))
        throw new Error("价格配置存在空值或无效数字，请修正后再执行");
      return applyPricingWithDraft(
        current as PricingConfig,
        pricingConfigWithRuleNames(snapshot.data.config),
        async (config) => {
          const saved = await api.updatePricingConfig(config);
          queryClient.setQueryData(["pricing"], saved);
          setDraft(pricingConfigWithRuleNames(saved.config));
        },
        api.applyPricing,
      );
    },
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
  const deleteBackup = useMutation({
    mutationFn: (backup: PricingBackup) => api.deletePricingBackup(backup.id),
    onSuccess: async (_result, deletedBackup) => {
      const remaining = (backups.data ?? []).filter((backup) => backup.id !== deletedBackup.id);
      queryClient.setQueryData(["pricing-backups"], remaining);
      setSelectedBackupID((selected) =>
        selected === deletedBackup.id ? (remaining[0]?.id ?? "") : selected,
      );
      setDeleteBackupTarget(null);
      await queryClient.invalidateQueries({ queryKey: ["pricing-backups"] });
      toast.success(`备份“${deletedBackup.name}”已删除`);
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : "价格分组备份删除失败"),
  });
  const current =
    draft ?? (snapshot.data ? pricingConfigWithRuleNames(snapshot.data.config) : null);
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
        ? pricingPreviewDecisions(
            snapshot.data.decisions,
            snapshot.data.groups,
            pricingConfigIsValid(current, snapshot.data.groups)
              ? (current as PricingConfig)
              : snapshot.data.config,
          )
        : [],
    [current, snapshot.data],
  );
  const running = Boolean(taskID) && !taskStopsPolling(task.data);
  const previewConfig =
    current && snapshot.data && pricingConfigIsValid(current, snapshot.data.groups)
      ? (current as PricingConfig)
      : null;
  const valid = previewConfig !== null;

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
    const setNumber = current.exchange_group_sets.length + 1;
    setDraft({
      ...current,
      exchange_group_sets: [...current.exchange_group_sets, []],
      exchange_group_set_names: [...current.exchange_group_set_names, `互换组 ${setNumber}`],
    });
  }

  function renameExchangeGroupSet(setIndex: number, name: string) {
    if (!current) return;
    const names = [...current.exchange_group_set_names];
    names[setIndex] = name;
    setDraft({ ...current, exchange_group_set_names: names });
  }

  function removeExchangeGroupSet(setIndex: number) {
    if (!current) return;
    setDraft({
      ...current,
      exchange_group_sets: current.exchange_group_sets.filter((_, index) => index !== setIndex),
      exchange_group_set_names: current.exchange_group_set_names.filter(
        (_, index) => index !== setIndex,
      ),
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
            <RefreshButton
              pending={snapshot.isFetching}
              ariaLabel="刷新价格数据"
              onClick={() => void snapshot.refetch()}
            />
            {props.page === "catalog" ? (
              <>
                <Button variant="outline" onClick={() => setChangesOpen(true)}>
                  <History /> 变更记录
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    setBackupName("");
                    setBackupDialog("create");
                  }}
                  disabled={!snapshot.data || !valid}
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
                  onClick={() => {
                    if (!current || !valid) {
                      toast.error("价格配置存在空值或无效数字，请修正后再保存");
                      return;
                    }
                    save.mutate(current as PricingConfig);
                  }}
                  disabled={!current || save.isPending}
                >
                  <Save /> {save.isPending ? "保存中" : "保存配置"}
                </Button>
                <Button
                  onClick={() => apply.mutate()}
                  disabled={!current?.enabled || !valid || running || apply.isPending}
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
              <div
                className="grid gap-3 px-3 py-2.5 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center lg:grid-cols-1 lg:items-start 2xl:grid-cols-[minmax(0,1fr)_8.5rem] 2xl:items-center"
                data-testid="pricing-goal-settings"
              >
                <FieldLabel
                  label="目标盈利比例"
                  description="利润 ÷ 账号成本；允许范围 0% - 99%"
                  htmlFor="pricing-profit-margin"
                />
                <span className="relative block">
                  <Input
                    id="pricing-profit-margin"
                    className="pr-8 tabular-nums"
                    type="number"
                    min={0}
                    max={99}
                    step="0.1"
                    value={
                      current.profit_margin === null
                        ? ""
                        : Number((current.profit_margin * 100).toFixed(4))
                    }
                    onChange={(event) =>
                      setDraft({
                        ...current,
                        profit_margin:
                          event.target.value === "" ? null : Number(event.target.value) / 100,
                      })
                    }
                  />
                  <span className="text-muted-foreground pointer-events-none absolute inset-y-0 right-2.5 flex items-center text-xs">
                    %
                  </span>
                </span>
              </div>
              <div
                className="grid gap-3 px-3 py-2.5 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center lg:grid-cols-1 lg:items-start 2xl:grid-cols-[minmax(0,1fr)_8.5rem] 2xl:items-center"
                data-testid="pricing-execution-settings"
              >
                <FieldLabel
                  label="动态调整间隔"
                  description="30 秒 - 24 小时"
                  htmlFor="pricing-interval-seconds"
                />
                <span className="relative block">
                  <Input
                    id="pricing-interval-seconds"
                    className="pr-9 tabular-nums"
                    type="number"
                    min={30}
                    max={86400}
                    value={current.interval_seconds ?? ""}
                    onChange={(event) =>
                      setDraft({
                        ...current,
                        interval_seconds:
                          event.target.value === "" ? null : Number(event.target.value),
                      })
                    }
                  />
                  <span className="text-muted-foreground pointer-events-none absolute inset-y-0 right-2.5 flex items-center text-xs">
                    秒
                  </span>
                </span>
              </div>
              <div className="grid gap-3 px-3 py-2.5 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center lg:grid-cols-1 lg:items-start 2xl:grid-cols-[minmax(0,1fr)_8.5rem] 2xl:items-center">
                <FieldLabel
                  label="写入并发"
                  description="允许范围 1 - 16"
                  htmlFor="pricing-write-concurrency"
                />
                <Input
                  id="pricing-write-concurrency"
                  className="tabular-nums"
                  type="number"
                  min={1}
                  max={16}
                  value={current.write_concurrency ?? ""}
                  onChange={(event) =>
                    setDraft({
                      ...current,
                      write_concurrency:
                        event.target.value === "" ? null : Number(event.target.value),
                    })
                  }
                />
              </div>
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
                    name={pricingRuleNameForInput(current.exchange_group_set_names, setIndex)}
                    groupSet={groupSet}
                    groups={snapshot.data.groups}
                    exchangeSetByGroup={exchangeSetByGroup}
                    exchangeSetNames={current.exchange_group_set_names}
                    onToggle={toggleExchangeGroup}
                    onNameChange={renameExchangeGroupSet}
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
      {previewConfig && snapshot.data ? (
        <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
          <DialogContent
            width="table"
            height="tall"
            className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
          >
            <DialogHeader>
              <DialogTitle>账号分组调整明细</DialogTitle>
              <DialogDescription>
                按目标盈利比例 {percent(previewConfig.profit_margin)} （利润 ÷
                账号成本）计算。默认只显示需要调整的账号，完整公式可在每行的计算明细中查看。
              </DialogDescription>
            </DialogHeader>
            <DialogBody className="overflow-hidden pr-0">
              <PricingPreviewTable
                decisions={previewDecisions}
                groups={snapshot.data.groups}
                config={previewConfig}
              />
            </DialogBody>
          </DialogContent>
        </Dialog>
      ) : null}
      <Dialog open={changesOpen} onOpenChange={setChangesOpen}>
        <DialogContent
          width="table"
          height="tall"
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>价格分组变更记录</DialogTitle>
            <DialogDescription>最近 100 批已完成的账号分组调整。</DialogDescription>
          </DialogHeader>
          <DialogBody className="overflow-hidden pr-0">
            {changes.error ? (
              <div className="flex h-full min-h-40 flex-col items-center justify-center gap-3">
                <QueryErrorToast error={changes.error} fallback="价格变更记录读取失败" />
                <span className="text-muted-foreground text-sm">价格变更记录读取失败</span>
                <RefreshButton
                  pending={changes.isFetching}
                  ariaLabel="刷新价格变更记录"
                  onClick={() => void changes.refetch()}
                />
              </div>
            ) : changes.isLoading ? (
              <div className="space-y-3" data-testid="pricing-changes-loading">
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-14 w-full" />
                <Skeleton className="h-14 w-full" />
              </div>
            ) : (
              <PricingChangeList records={changes.data ?? []} />
            )}
          </DialogBody>
        </DialogContent>
      </Dialog>
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
          if (!open && !deleteBackup.isPending) {
            setBackupDialog(null);
            setDeleteBackupTarget(null);
          }
        }}
      >
        <DialogContent height="medium" className="grid grid-rows-[auto_minmax(0,1fr)_auto]">
          <DialogHeader>
            <DialogTitle>{deleteBackupTarget ? "删除价格分组备份" : "从备份还原分组"}</DialogTitle>
            <DialogDescription>
              {deleteBackupTarget
                ? "只删除这份本地备份，不会修改当前账号分组；删除后无法恢复。"
                : "将批量改写管理平台账号分组。已不存在的账号或分组会跳过，人工优先账号不会被修改。"}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="grid content-start gap-2 pr-1">
            {deleteBackupTarget ? (
              <div className="flex items-center justify-between gap-3 rounded-lg border bg-muted/30 px-3 py-3">
                <span className="min-w-0 truncate font-medium">{deleteBackupTarget.name}</span>
                <Badge variant="secondary">{deleteBackupTarget.account_count} 个账号</Badge>
              </div>
            ) : (
              <>
                {backups.isError ? (
                  <p className="text-destructive text-sm">价格分组备份读取失败</p>
                ) : null}
                <PricingBackupList
                  backups={backups.data ?? []}
                  selectedBackupID={selectedBackupID}
                  onSelect={setSelectedBackupID}
                  onDelete={setDeleteBackupTarget}
                />
              </>
            )}
          </DialogBody>
          <DialogFooter>
            {deleteBackupTarget ? (
              <>
                <Button
                  variant="outline"
                  disabled={deleteBackup.isPending}
                  onClick={() => setDeleteBackupTarget(null)}
                >
                  取消
                </Button>
                <Button
                  variant="destructive"
                  disabled={deleteBackup.isPending}
                  onClick={() => deleteBackup.mutate(deleteBackupTarget)}
                >
                  <Trash2 aria-hidden="true" />
                  {deleteBackup.isPending ? "删除中" : "确认删除"}
                </Button>
              </>
            ) : (
              <>
                <Button variant="outline" onClick={() => setBackupDialog(null)}>
                  取消
                </Button>
                <Button
                  disabled={!selectedBackupID || restoreBackup.isPending || running}
                  onClick={() => restoreBackup.mutate(selectedBackupID)}
                >
                  <RotateCcw /> {restoreBackup.isPending ? "正在提交" : "确认还原"}
                </Button>
              </>
            )}
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
