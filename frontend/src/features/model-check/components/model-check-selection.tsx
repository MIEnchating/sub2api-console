import { CheckCheck, Cpu, Play, RefreshCw, Search, Timer, Users, X } from "lucide-react";

import type { AccountStatus } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

export type ModelCheckSelectionProps = {
  accounts: AccountStatus[];
  accountsLoading: boolean;
  accountsError: string | null;
  accountQuery: string;
  selectedAccountIDs: string[];
  models: string[];
  selectedModels: string[];
  modelsLoading: boolean;
  modelsError: string | null;
  rounds: number;
  timeoutSeconds: number;
  combinationCount: number;
  selectionError: string | null;
  disabled: boolean;
  canSubmit: boolean;
  onAccountQueryChange: (value: string) => void;
  onAccountToggle: (id: string, checked: boolean) => void;
  onAccountsSelectAll: () => void;
  onClear: () => void;
  onModelToggle: (model: string, checked: boolean) => void;
  onModelsSelectAll: () => void;
  onRefreshModels: () => void;
  onRoundsChange: (value: number) => void;
  onTimeoutChange: (value: number) => void;
  onSubmit: () => void;
};

function accountState(account: AccountStatus): string {
  if (account.paused === true) return "已暂停";
  if (account.schedulable === false) return "未调度";
  return account.health_status ?? account.health ?? "-";
}

function accountStateVariant(account: AccountStatus): "secondary" | "warning" | "outline" {
  if (account.paused === true || account.schedulable === false) return "warning";
  const state = accountState(account).toLocaleLowerCase();
  if (["健康", "正常", "healthy", "active"].some((value) => state.includes(value)))
    return "secondary";
  return "outline";
}

function modelProtocol(model: string): string {
  return model.startsWith("claude-") ? "Anthropic" : "Responses";
}

function AccountIdentity(props: { account: AccountStatus }) {
  return (
    <span className="block min-w-0">
      <span className="block truncate font-medium">{props.account.name}</span>
      <span className="text-muted-foreground block truncate text-xs tabular-nums">
        ID {props.account.id}
      </span>
    </span>
  );
}

function AccountMobileList(props: ModelCheckSelectionProps) {
  return (
    <div
      className="divide-y md:hidden"
      role="list"
      aria-label="账号列表"
      data-testid="model-check-account-mobile-list"
    >
      {props.accounts.map((account) => {
        const checked = props.selectedAccountIDs.includes(account.id);
        return (
          <label
            key={account.id}
            className={cn(
              "flex min-h-16 cursor-pointer items-center gap-3 px-3 py-2.5 transition-colors",
              checked ? "bg-primary/5" : "hover:bg-muted/40",
            )}
            role="listitem"
          >
            <Checkbox
              checked={checked}
              disabled={props.disabled || (!checked && props.selectedAccountIDs.length >= 20)}
              onCheckedChange={(next) => props.onAccountToggle(account.id, next)}
              aria-label={`选择账号 ${account.name}`}
            />
            <span className="min-w-0 flex-1">
              <AccountIdentity account={account} />
              <span className="text-muted-foreground mt-0.5 block truncate text-xs">
                {account.groups.join("、") || "未分组"}
              </span>
            </span>
            <span className="flex shrink-0 flex-col items-end gap-1">
              <Badge variant="outline">{account.platform ?? account.account_type ?? "-"}</Badge>
              <Badge variant={accountStateVariant(account)}>{accountState(account)}</Badge>
            </span>
          </label>
        );
      })}
    </div>
  );
}

function AccountEmptyState(props: ModelCheckSelectionProps) {
  if (props.accountsLoading) {
    return (
      <div className="text-muted-foreground grid min-h-40 place-items-center text-sm">
        正在读取账号
      </div>
    );
  }
  if (props.accountsError) {
    return (
      <div className="text-destructive grid min-h-40 place-items-center p-4 text-center text-sm">
        {props.accountsError}
      </div>
    );
  }
  if (props.accounts.length === 0) {
    return (
      <div className="text-muted-foreground grid min-h-40 place-items-center text-sm">
        没有匹配的账号
      </div>
    );
  }
  return null;
}

function AccountPanel(props: ModelCheckSelectionProps) {
  const emptyState = AccountEmptyState(props);
  return (
    <Card className="h-[30rem] gap-0 py-0 xl:h-[36rem]">
      <CardHeader className="grid-cols-[1fr_auto] items-center">
        <div className="min-w-0">
          <CardTitle>选择账号</CardTitle>
          <p className="text-muted-foreground mt-0.5 text-xs tabular-nums">
            已选 {props.selectedAccountIDs.length}/20 · 当前 {props.accounts.length} 个
          </p>
        </div>
        <div className="flex items-center gap-1.5">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={props.disabled || props.accounts.length === 0}
            onClick={props.onAccountsSelectAll}
          >
            <CheckCheck aria-hidden="true" />
            全选账号
          </Button>
          <Tooltip>
            <TooltipTrigger render={<span className="inline-flex" />}>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                disabled={
                  props.disabled ||
                  (props.selectedAccountIDs.length === 0 && props.selectedModels.length === 0)
                }
                onClick={props.onClear}
                aria-label="清空选择"
              >
                <X aria-hidden="true" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>清空选择</TooltipContent>
          </Tooltip>
        </div>
      </CardHeader>
      <div className="border-border/70 border-b p-2.5">
        <div className="relative">
          <Search
            className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2"
            aria-hidden="true"
          />
          <Input
            value={props.accountQuery}
            onChange={(event) => props.onAccountQueryChange(event.target.value)}
            placeholder="搜索账号、平台或分组"
            aria-label="搜索账号"
            className="pl-8"
          />
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {emptyState}
        {!emptyState && <AccountMobileList {...props} />}
        {!emptyState ? (
          <Table
            containerClassName="hidden min-h-0 md:block"
            className="min-w-[640px]"
            data-testid="model-check-account-desktop-table"
          >
            <TableHeader>
              <TableRow>
                <TableHead className="w-11">
                  <span className="sr-only">选择</span>
                </TableHead>
                <TableHead className="w-[32%]">账号</TableHead>
                <TableHead className="w-[18%]">平台</TableHead>
                <TableHead>分组</TableHead>
                <TableHead className="w-[18%]">状态</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.accounts.map((account) => {
                const checked = props.selectedAccountIDs.includes(account.id);
                return (
                  <TableRow key={account.id} data-state={checked ? "selected" : undefined}>
                    <TableCell overflowTooltip={false}>
                      <Checkbox
                        id={`model-check-account-${account.id}`}
                        checked={checked}
                        disabled={
                          props.disabled || (!checked && props.selectedAccountIDs.length >= 20)
                        }
                        onCheckedChange={(next) => props.onAccountToggle(account.id, next)}
                        aria-label={`选择账号 ${account.name}`}
                      />
                    </TableCell>
                    <TableCell>
                      <label
                        htmlFor={`model-check-account-${account.id}`}
                        className="block min-w-0 cursor-pointer"
                      >
                        <AccountIdentity account={account} />
                      </label>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">
                        {account.platform ?? account.account_type ?? "-"}
                      </Badge>
                    </TableCell>
                    <TableCell>{account.groups.join("、") || "-"}</TableCell>
                    <TableCell>
                      <Badge variant={accountStateVariant(account)}>{accountState(account)}</Badge>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        ) : null}
      </div>
    </Card>
  );
}

function ModelList(props: ModelCheckSelectionProps) {
  if (props.selectedAccountIDs.length === 0) {
    return (
      <div className="text-muted-foreground grid h-full min-h-32 place-items-center p-4 text-sm">
        请先选择账号
      </div>
    );
  }
  if (props.modelsLoading) {
    return (
      <div className="text-muted-foreground grid h-full min-h-32 place-items-center p-4 text-sm">
        正在读取共同模型
      </div>
    );
  }
  if (props.modelsError) {
    return (
      <div className="text-destructive grid h-full min-h-32 place-items-center p-4 text-center text-sm">
        {props.modelsError}
      </div>
    );
  }
  if (props.models.length === 0) {
    return (
      <div className="text-muted-foreground grid h-full min-h-32 place-items-center p-4 text-center text-sm">
        所选账号没有共同可检测模型
      </div>
    );
  }
  return (
    <div className="divide-y" role="list" aria-label="可检测模型">
      {props.models.map((model) => {
        const checked = props.selectedModels.includes(model);
        return (
          <label
            key={model}
            className={cn(
              "flex min-h-11 cursor-pointer items-center gap-3 px-3 py-2 transition-colors",
              checked ? "bg-primary/5" : "hover:bg-muted/40",
            )}
            role="listitem"
          >
            <Checkbox
              checked={checked}
              disabled={props.disabled || (!checked && props.selectedModels.length >= 20)}
              onCheckedChange={(next) => props.onModelToggle(model, next)}
              aria-label={`选择模型 ${model}`}
            />
            <Tooltip>
              <TooltipTrigger render={<span className="min-w-0 flex-1 truncate font-medium" />}>
                {model}
              </TooltipTrigger>
              <TooltipContent className="max-w-sm break-all">{model}</TooltipContent>
            </Tooltip>
            <Badge variant="outline">{modelProtocol(model)}</Badge>
          </label>
        );
      })}
    </div>
  );
}

function RoundControl(props: ModelCheckSelectionProps) {
  return (
    <div className="grid grid-cols-3 rounded-lg border p-0.5" aria-label="检测轮次">
      {[1, 2, 3].map((round) => (
        <Button
          key={round}
          type="button"
          size="sm"
          variant={props.rounds === round ? "secondary" : "ghost"}
          className="h-7"
          disabled={props.disabled}
          aria-pressed={props.rounds === round}
          onClick={() => props.onRoundsChange(round)}
        >
          {round} 轮
        </Button>
      ))}
    </div>
  );
}

function MatrixPanel(props: ModelCheckSelectionProps) {
  return (
    <Card className="h-[30rem] gap-0 py-0 xl:h-[36rem]">
      <CardHeader className="grid-cols-[1fr_auto] items-center">
        <div className="min-w-0">
          <CardTitle>检测矩阵</CardTitle>
          <p className="text-muted-foreground mt-0.5 text-xs tabular-nums">
            共同模型 · 已选 {props.selectedModels.length}/20
          </p>
        </div>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            size="xs"
            variant="ghost"
            disabled={props.disabled || props.models.length === 0}
            onClick={props.onModelsSelectAll}
          >
            全选
          </Button>
          <Tooltip>
            <TooltipTrigger render={<span className="inline-flex" />}>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                disabled={
                  props.disabled || props.selectedAccountIDs.length === 0 || props.modelsLoading
                }
                onClick={props.onRefreshModels}
                aria-label="刷新模型"
              >
                <RefreshCw
                  className={props.modelsLoading ? "animate-spin" : ""}
                  aria-hidden="true"
                />
              </Button>
            </TooltipTrigger>
            <TooltipContent>刷新模型</TooltipContent>
          </Tooltip>
        </div>
      </CardHeader>
      <CardContent className="min-h-0 flex-1 overflow-auto p-0!">
        <ModelList {...props} />
      </CardContent>
      <div className="border-border/70 shrink-0 border-t p-3">
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1 2xl:grid-cols-2">
          <div className="grid gap-1.5 text-xs font-medium">
            <span>检测轮次</span>
            <RoundControl {...props} />
          </div>
          <label className="grid gap-1.5 text-xs font-medium">
            请求超时
            <span className="relative block">
              <Timer
                className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2"
                aria-hidden="true"
              />
              <Input
                type="number"
                min={5}
                max={120}
                value={props.timeoutSeconds}
                disabled={props.disabled}
                onChange={(event) => props.onTimeoutChange(event.currentTarget.valueAsNumber)}
                className="pr-9 pl-8"
                aria-label="请求超时秒数"
              />
              <span className="text-muted-foreground pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-xs">
                秒
              </span>
            </span>
          </label>
        </div>
        <div className="border-border/70 my-3 grid grid-cols-3 border-y py-2.5 text-center">
          <div>
            <Users className="text-muted-foreground mx-auto mb-1 size-3.5" aria-hidden="true" />
            <strong className="block text-base leading-none tabular-nums">
              {props.selectedAccountIDs.length}
            </strong>
            <span className="text-muted-foreground text-[11px]">账号</span>
          </div>
          <div className="border-border/70 border-x">
            <Cpu className="text-muted-foreground mx-auto mb-1 size-3.5" aria-hidden="true" />
            <strong className="block text-base leading-none tabular-nums">
              {props.selectedModels.length}
            </strong>
            <span className="text-muted-foreground text-[11px]">模型</span>
          </div>
          <div>
            <CheckCheck
              className="text-muted-foreground mx-auto mb-1 size-3.5"
              aria-hidden="true"
            />
            <strong className="block text-base leading-none tabular-nums">
              {props.combinationCount}
            </strong>
            <span className="text-muted-foreground text-[11px]">组合</span>
          </div>
        </div>
        {props.selectionError ? (
          <p className="text-destructive mb-2 text-xs" role="alert">
            {props.selectionError}
          </p>
        ) : null}
        <Button
          type="button"
          className="w-full"
          disabled={!props.canSubmit}
          onClick={props.onSubmit}
        >
          {props.disabled ? (
            <RefreshCw className="animate-spin" aria-hidden="true" />
          ) : (
            <Play aria-hidden="true" />
          )}
          {props.disabled
            ? "检测中"
            : `开始检测${props.combinationCount > 0 ? ` · ${props.combinationCount} 个组合` : ""}`}
        </Button>
      </div>
    </Card>
  );
}

export function ModelCheckSelection(props: ModelCheckSelectionProps) {
  return (
    <section
      className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_21rem]"
      aria-label="模型检测配置"
      data-testid="model-check-selection-layout"
    >
      <AccountPanel {...props} />
      <MatrixPanel {...props} />
    </section>
  );
}
