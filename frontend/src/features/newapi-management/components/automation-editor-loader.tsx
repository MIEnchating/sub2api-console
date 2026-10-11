import type { ReactElement } from "react";
import { useAutomationChannelCatalog } from "../hooks/use-automation-channel-catalog";
import { type AccountModelAutomationSchedule } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AccountModelAutomationDialog } from "./account-model-automation-dialog";

export function AutomationEditorLoader(props: {
  platformId: string;
  schedule?: AccountModelAutomationSchedule;
  onOpenChange: (open: boolean) => void;
}): ReactElement {
  const catalog = useAutomationChannelCatalog(
    props.platformId,
    props.schedule?.request.channels.map((channel) => channel.id) ?? [],
  );
  return (
    <Dialog open onOpenChange={props.onOpenChange}>
      <DialogContent width="wide" height="adaptive">
        {catalog.pageError && (
          <QueryErrorToast error={catalog.pageError} fallback="渠道列表读取失败" />
        )}
        {catalog.selectionError && (
          <QueryErrorToast error={catalog.selectionError} fallback="已选渠道读取失败" />
        )}
        {catalog.opened ? (
          <AccountModelAutomationDialog
            open
            withinDialog
            platformId={props.platformId}
            channels={catalog.channels}
            channelIDs={catalog.selectedIDs}
            onChannelIDsChange={catalog.changeSelection}
            channelPageControls={catalog.pageControls}
            channelsReady={catalog.ready}
            channelsPending={catalog.pending}
            onRefreshChannels={catalog.refresh}
            schedule={props.schedule}
            onOpenChange={props.onOpenChange}
          />
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>{props.schedule ? "编辑运营任务" : "添加运营任务"}</DialogTitle>
              <DialogDescription>读取渠道后配置账号分组和运行方式。</DialogDescription>
            </DialogHeader>
            <DialogBody>
              {catalog.pending ? (
                <ContentLoading label="正在读取渠道" />
              ) : (
                <ContentRetry pending={catalog.pending} onRetry={() => catalog.refresh()} />
              )}
            </DialogBody>
            <DialogFooter>
              <Button variant="outline" onClick={() => props.onOpenChange(false)}>
                取消
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
