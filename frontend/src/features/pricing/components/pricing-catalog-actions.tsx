import type { ReactElement } from "react";
import { DatabaseBackup, Eye, History, RotateCcw } from "lucide-react";

import { PageActionMenu } from "@/components/page-action-menu";
import { Button } from "@/components/ui/button";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";

export function PricingCatalogActions(props: {
  previewDisabled: boolean;
  backupDisabled: boolean;
  restoreDisabled: boolean;
  onPreview: () => void;
  onHistory: () => void;
  onBackup: () => void;
  onRestore: () => void;
}): ReactElement {
  return (
    <>
      <Button
        onClick={props.onPreview}
        disabled={props.previewDisabled}
        aria-label="查看账号调整明细"
      >
        <Eye aria-hidden="true" />
        <span className="hidden sm:inline">查看账号调整明细</span>
      </Button>
      <PageActionMenu label="价格维护">
        <DropdownMenuItem onClick={props.onHistory}>
          <History aria-hidden="true" /> 变更记录
        </DropdownMenuItem>
        <DropdownMenuItem disabled={props.backupDisabled} onClick={props.onBackup}>
          <DatabaseBackup aria-hidden="true" /> 创建备份
        </DropdownMenuItem>
        <DropdownMenuItem disabled={props.restoreDisabled} onClick={props.onRestore}>
          <RotateCcw aria-hidden="true" /> 从备份还原
        </DropdownMenuItem>
      </PageActionMenu>
    </>
  );
}
