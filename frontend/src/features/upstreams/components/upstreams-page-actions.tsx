import type { ReactElement } from "react";
import {
  ChartNoAxesColumnIncreasing,
  FolderOpen,
  ScanSearch,
  Server,
  SpellCheck2,
  UserPlus,
  WalletCards,
} from "lucide-react";

import { PageActions } from "@/components/page-actions";
import { PageActionMenu } from "@/components/page-action-menu";
import { RefreshButton } from "@/components/refresh-button";
import { Button } from "@/components/ui/button";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";

export function UpstreamsPageActions(props: {
  refreshing: boolean;
  maintenancePending: boolean;
  syncPending: boolean;
  auditPending: boolean;
  auditDisabled: boolean;
  onRefresh: () => void;
  onAdd: () => void;
  onHistory: () => void;
  onBalanceSync: () => void;
  onNameRepair: () => void;
  onGroupSync: () => void;
  onGroupAudit: () => void;
  onSync: () => void;
}): ReactElement {
  return (
    <PageActions>
      <RefreshButton
        pending={props.refreshing}
        ariaLabel="刷新上游列表"
        onClick={props.onRefresh}
      />
      <Button onClick={props.onAdd} aria-label="添加账号">
        <UserPlus aria-hidden="true" />
        <span className="hidden sm:inline">添加账号</span>
      </Button>
      <PageActionMenu label="上游维护">
        <DropdownMenuItem onClick={props.onHistory}>
          <ChartNoAxesColumnIncreasing aria-hidden="true" /> 统计变化
        </DropdownMenuItem>
        <DropdownMenuItem disabled={props.syncPending} onClick={props.onSync}>
          <Server aria-hidden="true" /> 同步上游
        </DropdownMenuItem>
        <DropdownMenuItem disabled={props.maintenancePending} onClick={props.onBalanceSync}>
          <WalletCards aria-hidden="true" /> 同步余额
        </DropdownMenuItem>
        <DropdownMenuItem disabled={props.maintenancePending} onClick={props.onNameRepair}>
          <SpellCheck2 aria-hidden="true" /> 名称修复
        </DropdownMenuItem>
        <DropdownMenuItem disabled={props.maintenancePending} onClick={props.onGroupSync}>
          <FolderOpen aria-hidden="true" /> 同步分组
        </DropdownMenuItem>
        <DropdownMenuItem disabled={props.auditDisabled} onClick={props.onGroupAudit}>
          <ScanSearch aria-hidden="true" /> {props.auditPending ? "核对中…" : "核对分组绑定"}
        </DropdownMenuItem>
      </PageActionMenu>
    </PageActions>
  );
}
