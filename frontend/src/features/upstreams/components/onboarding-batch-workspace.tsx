import type { ReactNode } from "react";
import { Eye, ExternalLink } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { OnboardingEntryKind } from "@/lib/onboarding-entry";

export function onboardingSelectionLayout(
  entryKind: OnboardingEntryKind,
  upstreamVerified: boolean,
): {
  fixedContent: boolean;
  cardClassName: string | undefined;
  contentClassName: string;
  preparedClassName: string;
  tablePanelClassName: string;
  tableContainerClassName: string;
} {
  const fixedContent = entryKind === "host" || (entryKind === "full" && upstreamVerified);
  let cardClassName: string | undefined;
  if (fixedContent) {
    cardClassName = "h-full min-h-0";
  } else if (entryKind === "full") {
    cardClassName = "mt-3 sm:mt-4";
  }
  return {
    fixedContent,
    cardClassName,
    contentClassName: fixedContent ? "grid min-h-0 flex-1 gap-3 overflow-hidden" : "grid gap-3",
    preparedClassName: fixedContent ? "flex min-h-0 flex-col gap-3 overflow-hidden" : "grid gap-3",
    tablePanelClassName: fixedContent
      ? "flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border"
      : "overflow-hidden rounded-lg border",
    tableContainerClassName: fixedContent
      ? "min-h-0 flex-1 overflow-auto"
      : "overflow-x-auto overflow-y-hidden",
  };
}

export function OnboardingStepIndicator(props: { completed: boolean }) {
  if (props.completed) return null;
  return (
    <div className="mb-3 grid grid-cols-2 overflow-hidden rounded-lg border sm:mb-4">
      <div className="bg-muted/50 flex items-center gap-2 px-3 py-2.5 text-sm font-medium">
        <span className="flex size-6 items-center justify-center rounded-full border">1</span>
        添加上游并完成鉴权
      </div>
      <div className="text-muted-foreground flex items-center gap-2 border-l px-3 py-2.5 text-sm">
        <span className="flex size-6 items-center justify-center rounded-full border">2</span>
        选择分组并添加账号
      </div>
    </div>
  );
}

export function OnboardingCandidateIdentity(props: {
  groupName: string;
  platformLabel: string | null;
  description: string | null;
  status: ReactNode;
}) {
  return (
    <div className="grid min-w-0 gap-1">
      <div className="flex min-w-0 items-center gap-2">
        <span className="truncate font-medium">{props.groupName}</span>
        {props.status}
      </div>
      <div className="text-muted-foreground flex min-w-0 items-center gap-2 text-xs">
        <span className="shrink-0">{props.platformLabel ?? "未知"}</span>
        <span aria-hidden="true">·</span>
        <Tooltip>
          <TooltipTrigger render={<span className="truncate" />}>
            {props.description ?? "未提供说明"}
          </TooltipTrigger>
          <TooltipContent className="max-w-sm break-words">
            {props.description ?? "未提供说明"}
          </TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}

export function OnboardingInferredPlatformStatus(props: {
  required: boolean;
  selectedCount: number;
  platformLabel: string | null;
}) {
  if (!props.required || props.selectedCount === 0) return null;
  if (!props.platformLabel) {
    return (
      <span role="status" className="text-warning text-xs">
        所选本地分组无法唯一确定账号类型
      </span>
    );
  }
  return (
    <span role="status" className="text-muted-foreground text-xs">
      账号类型：<strong className="text-foreground font-medium">{props.platformLabel}</strong>
      （由本地分组确定）
    </span>
  );
}

export function OnboardingAccountType(props: {
  required: boolean;
  selectedCount: number;
  platformLabel: string | null;
}) {
  let label = props.platformLabel;
  if (props.required && props.selectedCount === 0) label = null;
  return (
    <span className={label ? "text-sm font-medium" : "text-muted-foreground text-sm"}>
      {label ?? "待选择分组"}
    </span>
  );
}

export function OnboardingUpstreamSummary(props: {
  name: string;
  baseUrl: string;
  typeLabel: string;
  balance: string;
  rechargeRatio: string;
  selectableCount: number;
  boundCount: number;
  controls?: ReactNode;
}) {
  return (
    <section
      aria-label="当前上游概况"
      className="flex min-w-0 flex-wrap items-center gap-x-5 gap-y-2 border-b pb-3"
    >
      <div className="flex min-w-0 items-center gap-2">
        <a
          href={props.baseUrl}
          target="_blank"
          rel="noreferrer"
          className="text-primary inline-flex min-w-0 items-center gap-1 font-medium hover:underline"
          aria-label={`访问上游 ${props.name}`}
        >
          <span className="truncate">{props.name}</span>
          <ExternalLink className="size-3.5 shrink-0" aria-hidden="true" />
        </a>
        <Badge variant="outline">{props.typeLabel}</Badge>
      </div>
      <dl className="flex flex-wrap items-center gap-x-5 gap-y-1 text-sm tabular-nums">
        <div className="flex items-center gap-1.5">
          <dt className="text-muted-foreground">余额</dt>
          <dd className="font-medium">{props.balance}</dd>
        </div>
        <div className="flex items-center gap-1.5">
          <dt className="text-muted-foreground">充值比例</dt>
          <dd className="font-medium">{props.rechargeRatio}</dd>
        </div>
      </dl>
      <div className="flex min-w-0 flex-wrap items-center gap-3 sm:ml-auto">
        <div className="flex items-center gap-2 text-xs tabular-nums">
          <span className="bg-muted rounded-md px-2 py-1 font-medium">
            {props.selectableCount} 个可选
          </span>
          <span className="text-muted-foreground rounded-md border px-2 py-1">
            {props.boundCount} 个已绑定
          </span>
        </div>
        {props.controls}
      </div>
    </section>
  );
}

export function OnboardingBatchActionBar(props: {
  controls: ReactNode;
  selectedCount: number;
  pending: boolean;
  disabled: boolean;
  onSubmit: () => void;
}) {
  return (
    <div
      role="toolbar"
      aria-label="批量添加账号"
      className="bg-card/95 sticky bottom-0 z-20 -mx-1 flex flex-col gap-3 border-t px-1 pt-3 pb-1 backdrop-blur-sm lg:flex-row lg:items-end"
    >
      <div className="grid min-w-0 flex-1 gap-3 sm:grid-cols-3">{props.controls}</div>
      <div className="flex shrink-0 items-center justify-between gap-3 lg:justify-end">
        <span className="text-muted-foreground text-sm tabular-nums" aria-live="polite">
          {props.selectedCount} 项待提交
        </span>
        <Button type="button" disabled={props.disabled || props.pending} onClick={props.onSubmit}>
          <Eye aria-hidden="true" />
          {props.pending ? "正在提交" : `预览 ${props.selectedCount} 项变更`}
        </Button>
      </div>
    </div>
  );
}
