import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type KumaTemplate } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { kumaMonitorsKey, kumaTemplatesKey } from "../constants";
import { useTaskCompletion } from "../hooks/use-task-completion";
import { OperationDialogFooter } from "./operation-dialog-footer";
import { TemplateDistributionSelection } from "./template-distribution-selection";
import { TemplateDistributionResults } from "./template-distribution-results";
import { isKumaTaskCancellation } from "../lib/task-cancellation";

export function TemplateDistributionDialog(props: { item: KumaTemplate; onClose: () => void }) {
  const client = useQueryClient();
  const completion = useTaskCompletion();
  const [selection, setSelection] = useState<number[] | null>(null);
  const preview = useQuery({
    queryKey: ["uptime-kuma", "template-distribution", props.item.id, props.item.revision],
    queryFn: ({ signal }) =>
      api.kumaTemplateDistribution(props.item.id, props.item.revision, signal),
    retry: false,
    gcTime: 0,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
  const eligibleItems = preview.data?.items.filter((item) => !item.reason) ?? [];
  const selectedIDs = eligibleItems
    .filter((item) => selection === null || selection.includes(item.monitor_id))
    .map((item) => item.monitor_id);
  const mutation = useMutation({
    mutationFn: async () => {
      if (!preview.data) throw new Error("请先读取影响范围");
      if (!selectedIDs.length) throw new Error("请至少选择一个可下发的监控项");
      completion.prepare();
      return completion.wait(
        await api.distributeKumaTemplate(props.item.id, {
          revision: preview.data.revision,
          scope: preview.data.scope,
          monitor_ids: selectedIDs,
        }),
      );
    },
    onSuccess: () => toast.success("模板参数已下发"),
    onError: (error) => {
      if (isKumaTaskCancellation(error)) toast.info(error.message);
      else notifyOperationError(error, "参数下发失败");
    },
    onSettled: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: kumaMonitorsKey }),
        client.invalidateQueries({ queryKey: kumaTemplatesKey }),
      ]);
    },
  });
  const submitted = mutation.isSuccess || mutation.isError;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) props.onClose();
      }}
    >
      <DialogContent
        width="wide"
        showCloseButton={!mutation.isPending}
        className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      >
        <DialogHeader>
          <DialogTitle>参数下发</DialogTitle>
          <DialogDescription>
            将「{props.item.name}」第 {props.item.revision}{" "}
            版参数下发到选中的关联监控项。默认全选可下发项，请核对后确认。
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <p className="text-sm text-muted-foreground">
            更新检测间隔、超时、重试、请求方法、请求头、请求体及其他检测参数。保留各监控项的名称、类型、目标地址（含主机和端口）、分组、启停状态、通知关联、独立鉴权和模型覆盖。模板中的鉴权设置不会下发。
          </p>
          {preview.isPending && <ContentLoading label="正在读取参数下发影响范围…" />}
          {preview.error && <QueryErrorToast error={preview.error} fallback="影响范围读取失败" />}
          {preview.isError && (
            <ContentRetry pending={preview.isFetching} onRetry={() => void preview.refetch()} />
          )}
          {preview.data && (
            <TemplateDistributionSelection
              preview={preview.data}
              selectedIDs={selectedIDs}
              disabled={mutation.isPending || submitted || preview.isFetching || preview.isError}
              onChange={setSelection}
            />
          )}
          {submitted && <TemplateDistributionResults task={completion.result} />}
        </DialogBody>
        <OperationDialogFooter
          pending={mutation.isPending}
          starting={completion.starting}
          task={completion.task}
        >
          <Button variant="outline" disabled={mutation.isPending} onClick={props.onClose}>
            {submitted ? "关闭" : "取消"}
          </Button>
          {!submitted && (
            <Button
              disabled={
                mutation.isPending ||
                preview.isFetching ||
                preview.isError ||
                selectedIDs.length === 0
              }
              onClick={() => mutation.mutate()}
            >
              {mutation.isPending ? "正在下发…" : "确认下发"}
            </Button>
          )}
        </OperationDialogFooter>
      </DialogContent>
    </Dialog>
  );
}
