import type { ReactElement } from "react";
import {
  Activity,
  BadgeCheck,
  ChevronDown,
  RefreshCw,
  ScanSearch,
  SpellCheck2,
} from "lucide-react";
import { PageActions } from "@/components/page-actions";
import { RefreshButton } from "@/components/refresh-button";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export function AccountsPageActions(props: {
  refreshing: boolean;
  automaticDisabled: boolean;
  rateSyncDisabled: boolean;
  onRefresh: () => void;
  onProbe: () => void;
  onCheck: () => void;
  onRateSync: () => void;
  onModelSync: () => void;
  onRevalidate: () => void;
  onRepairNames: () => void;
}): ReactElement {
  return (
    <PageActions>
      <RefreshButton pending={props.refreshing} ariaLabel="刷新账号池" onClick={props.onRefresh} />
      <Button onClick={props.onProbe} aria-label="平台模型探活">
        <Activity aria-hidden="true" />
        <span className="hidden sm:inline">平台模型探活</span>
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button variant="outline" />}>
          账号维护 <ChevronDown aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          <DropdownMenuItem disabled={props.automaticDisabled} onClick={props.onCheck}>
            <ScanSearch aria-hidden="true" />
            配置校验与修复
          </DropdownMenuItem>
          <DropdownMenuItem disabled={props.rateSyncDisabled} onClick={props.onRateSync}>
            <RefreshCw aria-hidden="true" />
            同步倍率
          </DropdownMenuItem>
          <DropdownMenuItem disabled={props.automaticDisabled} onClick={props.onModelSync}>
            <RefreshCw aria-hidden="true" />
            同步模型
          </DropdownMenuItem>
          <DropdownMenuItem disabled={props.automaticDisabled} onClick={props.onRevalidate}>
            <BadgeCheck aria-hidden="true" />
            复验绑定
          </DropdownMenuItem>
          <DropdownMenuItem disabled={props.automaticDisabled} onClick={props.onRepairNames}>
            <SpellCheck2 aria-hidden="true" />
            命名修复
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </PageActions>
  );
}
