import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeftRight,
  ChevronDown,
  CircleDollarSign,
  Eye,
  Play,
  Plus,
  RefreshCw,
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
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

function groupPurchaseMultipliers(groupID: string, decisions: PricingDecision[]) {
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

function GroupPurchaseMultiplierCell(props: { groupID: string; decisions: PricingDecision[] }) {
  const summary = groupPurchaseMultipliers(props.groupID, props.decisions);
  if (summary.accounts === 0) return <span className="text-muted-foreground">无账号</span>;
  if (summary.values.length === 0) {
    return (
      <div className="grid gap-0.5">
        <span className="text-muted-foreground">未记录</span>
        <span className="text-muted-foreground text-xs">{summary.accounts} 个账号</span>
      </div>
    );
  }
  const label =
    summary.values.length === 1
      ? summary.values[0]
      : `${summary.values[0]} - ${summary.values[summary.values.length - 1]}`;
  const content = `当前 ${summary.accounts} 个账号，进货倍率共 ${summary.values.length} 档：${summary.values.join("、")}`;
  return (
    <div className="grid gap-0.5 tabular-nums">
      {summary.values.length === 1 ? (
        <span className="font-medium">{label}</span>
      ) : (
        <Tooltip>
          <TooltipTrigger
            render={
              <button
                type="button"
                className="w-fit border-b border-dashed font-medium"
                aria-label={`查看分组 ${props.groupID} 的全部进货倍率`}
              />
            }
          >
            {label}
          </TooltipTrigger>
          <TooltipContent className="max-w-sm whitespace-normal">{content}</TooltipContent>
        </Tooltip>
      )}
      <span className="text-muted-foreground text-xs">
        {summary.accounts} 个账号{summary.values.length > 1 ? ` · ${summary.values.length} 档` : ""}
      </span>
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

function pricingDecisionBasis(
  decision: PricingDecision,
  groups: PricingGroup[],
  config: PricingConfig,
) {
  if (decision.skipped)
    return decision.reason ? [decision.reason] : ["账号资料不足，无法计算目标分组"];
  const cost = Number(decision.cost_multiplier);
  if (!Number.isFinite(cost) || cost <= 0) return ["账号成本倍率无效，保留当前分组"];
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
        `${name}：进货倍率 ${decision.cost_multiplier} ${cost <= limit ? "≤" : ">"} 可接受上限 ${decimal(limit)}（售价 ${group.rate_multiplier} × ${percent(1 - config.profit_margin)}）`,
      );
    }
  }
  return rows;
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
    decision.current_group_ids.forEach((groupID) => {
      const setIndex = setByGroup.get(groupID);
      if (setIndex !== undefined) activeSets.add(setIndex);
    });
    const desired = decision.current_group_ids.filter((groupID) => {
      const group = byID.get(groupID);
      return !setByGroup.has(groupID) || !group?.available || group.platform !== decision.platform;
    });
    const eligible: string[] = [];
    for (const setIndex of activeSets) {
      for (const groupID of config.exchange_group_sets[setIndex]) {
        const group = byID.get(groupID);
        const rate = Number(group?.rate_multiplier);
        if (!group?.available || group.platform !== decision.platform || !Number.isFinite(rate))
          continue;
        if (cost <= rate * (1 - config.profit_margin)) {
          desired.push(groupID);
          eligible.push(group.name);
        }
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

export function PricingPreviewTable(props: {
  decisions: PricingDecision[];
  groups: PricingGroup[];
  config: PricingConfig;
}) {
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const totalPages = Math.max(1, Math.ceil(props.decisions.length / pageSize));
  const page = Math.min(currentPage, totalPages);
  const visibleDecisions = props.decisions.slice((page - 1) * pageSize, page * pageSize);
  const groupNames = useMemo(
    () => new Map(props.groups.map((group) => [group.id, group.name])),
    [props.groups],
  );

  useEffect(() => {
    if (currentPage > totalPages) setCurrentPage(totalPages);
  }, [currentPage, totalPages]);

  return (
    <div
      className="flex h-full min-h-0 flex-col overflow-hidden rounded-md border"
      data-testid="pricing-preview-table"
    >
      <Table
        className="min-w-[82rem]"
        containerClassName="min-h-0 flex-1 overflow-auto"
        overflowTooltip={false}
      >
        <TableHeader>
          <TableRow>
            <TableHead className="w-52">账号</TableHead>
            <TableHead className="w-28">进货倍率</TableHead>
            <TableHead className="w-44">当前分组</TableHead>
            <TableHead className="w-44">调整后分组</TableHead>
            <TableHead className="w-56">具体变更</TableHead>
            <TableHead>判定依据</TableHead>
            <TableHead className="w-28">状态</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.decisions.length === 0 ? (
            <TableRow>
              <TableCell colSpan={7} className="text-muted-foreground h-24 text-center">
                当前没有账号价格数据
              </TableCell>
            </TableRow>
          ) : (
            visibleDecisions.map((decision) => {
              const changes = pricingGroupChanges(decision);
              const basis = pricingDecisionBasis(decision, props.groups, props.config);
              return (
                <TableRow key={decision.account_id}>
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
                    <span className="break-words">
                      {groupIDsLabel(decision.current_group_ids, groupNames)}
                    </span>
                  </TableCell>
                  <TableCell className="align-top whitespace-normal">
                    <span className="break-words">
                      {groupIDsLabel(decision.desired_group_ids, groupNames)}
                    </span>
                  </TableCell>
                  <TableCell className="align-top whitespace-normal">
                    {decision.skipped ? (
                      <span className="text-muted-foreground text-xs">不会修改账号分组</span>
                    ) : decision.changed ? (
                      <div className="grid gap-1 text-xs leading-5">
                        {changes.added.length > 0 ? (
                          <span className="text-success break-words">
                            加入：{groupIDsLabel(changes.added, groupNames)}
                          </span>
                        ) : null}
                        {changes.removed.length > 0 ? (
                          <span className="text-destructive break-words">
                            移出：{groupIDsLabel(changes.removed, groupNames)}
                          </span>
                        ) : null}
                      </div>
                    ) : (
                      <span className="text-muted-foreground text-xs">保留当前分组</span>
                    )}
                  </TableCell>
                  <TableCell className="align-top whitespace-normal">
                    <div className="grid gap-1 text-xs leading-5">
                      {basis.map((line) => (
                        <span className="text-muted-foreground break-words" key={line}>
                          {line}
                        </span>
                      ))}
                    </div>
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
                </TableRow>
              );
            })
          )}
        </TableBody>
      </Table>
      <DataTablePagination
        currentPage={page}
        totalPages={totalPages}
        totalItems={props.decisions.length}
        pageSize={pageSize}
        pageSizes={[10, 20, 50, 100]}
        onPageChange={setCurrentPage}
        onPageSizeChange={(value) => {
          setPageSize(value);
          setCurrentPage(1);
        }}
      />
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
      className="border-t"
      data-testid={`exchange-set-${setNumber}`}
      aria-labelledby={`exchange-set-title-${setNumber}`}
    >
      <div className="bg-muted/20 flex min-h-11 items-center justify-between gap-3 px-4 py-2.5">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span id={`exchange-set-title-${setNumber}`} className="font-medium">
            互换组 {setNumber}
          </span>
          <Badge variant={complete ? "outline" : "warning"}>{statusLabel}</Badge>
          {selectedPlatform ? <Badge variant="secondary">{selectedPlatform}</Badge> : null}
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

      <div id={contentID} className="space-y-4 px-4 py-3.5" hidden={!expanded}>
        {[...sections.entries()].map(([platform, groups]) => (
          <div key={platform} data-platform-section={platform} className="space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground text-xs font-medium">{platform}</span>
              <span className="bg-border h-px min-w-4 flex-1" aria-hidden="true" />
              <span className="text-muted-foreground text-xs tabular-nums">
                {groups.filter((group) => group.available).length} 个可用
              </span>
            </div>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
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
                    data-selected={selected ? "true" : "false"}
                    className={cn(
                      "flex min-w-0 items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors",
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
  const previewChanges = previewDecisions.filter((item) => item.changed).length;
  const previewSkipped = previewDecisions.filter((item) => item.skipped).length;
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
    <PageLayout>
      <PageHeading
        eyebrow={props.page === "catalog" ? "OPERATIONS / PRICING" : "POLICY / PRICING"}
        title={props.page === "catalog" ? "价格管理" : "价格配置"}
        description={
          props.page === "catalog"
            ? "查看各业务分组当前使用的售价倍率。"
            : "配置自动调价参数、账号互换范围与执行策略。"
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
              <Button
                variant="outline"
                onClick={() => setPreviewOpen(true)}
                disabled={!snapshot.data}
              >
                <Eye /> 查看账号调整明细
              </Button>
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
        <div data-testid="pricing-page">
          <Card size="sm" className="overflow-hidden">
            <CardHeader>
              <CardTitle>分组售价</CardTitle>
              <CardDescription>价格数据来自当前分组配置。</CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>分组</TableHead>
                    <TableHead className="w-40">平台</TableHead>
                    <TableHead className="w-40">进货倍率</TableHead>
                    <TableHead className="w-40">售价倍率</TableHead>
                    <TableHead className="w-48">状态</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {snapshot.data.groups.map((group) => (
                    <TableRow key={group.id}>
                      <TableCell>
                        <span className="font-medium">{group.name}</span>
                        <span className="text-muted-foreground ml-2">#{group.id}</span>
                      </TableCell>
                      <TableCell>{group.platform || "-"}</TableCell>
                      <TableCell>
                        <GroupPurchaseMultiplierCell
                          groupID={group.id}
                          decisions={snapshot.data.decisions}
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
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      ) : current ? (
        <div className="space-y-3" data-testid="pricing-config-page">
          <Card size="sm">
            <CardHeader className="grid-cols-[1fr_auto]">
              <div className="flex min-w-0 items-center gap-3">
                <span className="bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md">
                  <CircleDollarSign className="size-4" aria-hidden="true" />
                </span>
                <div className="min-w-0">
                  <CardTitle>自动价格分组</CardTitle>
                  <CardDescription>
                    成本不高于售价扣除目标利润后的账号进入对应分组。
                  </CardDescription>
                </div>
              </div>
              <div className="flex items-center gap-2">
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
              <label className="grid gap-3 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center lg:grid-cols-1 lg:items-start 2xl:grid-cols-[minmax(0,1fr)_9rem] 2xl:items-center">
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
              <label className="grid gap-3 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center lg:grid-cols-1 lg:items-start 2xl:grid-cols-[minmax(0,1fr)_9rem] 2xl:items-center">
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
              <label className="grid gap-3 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center lg:grid-cols-1 lg:items-start 2xl:grid-cols-[minmax(0,1fr)_9rem] 2xl:items-center">
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
              <Button variant="outline" onClick={addExchangeGroupSet}>
                <Plus /> 添加互换组
              </Button>
            </CardHeader>
            <CardContent className="p-0">
              {current.exchange_group_sets.length === 0 ? (
                <div
                  className="flex min-h-28 flex-wrap items-center justify-center gap-x-4 gap-y-3 border-t px-4 py-5"
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
                按已保存的目标盈利比例 {percent(current.profit_margin)} 计算，共{" "}
                {previewDecisions.length} 个账号：将调整 {previewChanges} 个，无需调整{" "}
                {previewDecisions.length - previewChanges - previewSkipped} 个，无法判定{" "}
                {previewSkipped} 个。表格会明确列出加入、移出的分组以及每个售价分组的成本上限。
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
    </PageLayout>
  );
}

export function PricingPage() {
  return <PricingWorkspace page="catalog" />;
}

export function PricingConfigPage() {
  return <PricingWorkspace page="config" />;
}
