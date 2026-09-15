import { useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { api, type WorkbenchSMSReceipt, type WorkbenchScope } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { smsReceiptActionLabels, smsReceiptStateLabels, workbenchKeys } from "../constants";
import { smsProviderOptions } from "../lib/oauth-sms-schema";
import { WorkbenchSMSInspect } from "./workbench-sms-inspect";

export function WorkbenchSMSReceipts(
  props: { scope?: WorkbenchScope; taskId?: string } = {},
): ReactElement {
  const [selected, setSelected] = useState<WorkbenchSMSReceipt | null>(null);
  const query = useQuery({
    queryKey: [...workbenchKeys.smsReceipts, props.scope ?? "managed"],
    queryFn: (context) => api.workbenchSMSReceipts(context.signal, props.scope),
    gcTime: 0,
  });
  const receipts = query.data?.filter((item) => !props.taskId || item.task_id === props.taskId);
  return (
    <section aria-label="短信订单记录" className="grid min-w-0 gap-3">
      <h2 className="text-base font-medium">短信订单记录</h2>
      {query.isPending && (
        <div role="status" aria-label="正在读取短信订单记录" className="grid gap-3">
          <Skeleton className="h-20" />
          <Skeleton className="h-20" />
        </div>
      )}
      {query.isError && (
        <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
      )}
      {receipts?.length === 0 && <p className="text-sm text-muted-foreground">暂无短信订单记录</p>}
      <ul className="divide-y">
        {receipts?.map((receipt) => (
          <li
            key={receipt.id}
            className="flex min-w-0 flex-wrap items-center justify-between gap-3 py-3"
          >
            <div className="min-w-0 space-y-1 text-sm wrap-anywhere">
              <p>
                {smsProviderOptions.find((item) => item.value === receipt.provider)?.label} ·{" "}
                {smsReceiptActionLabels[receipt.action]} · {smsReceiptStateLabels[receipt.state]}
              </p>
              <p>
                {receipt.order_id || "供应商订单 ID 未确认"} · {receipt.phone || "号码未确认"}
              </p>
              <p className="text-xs text-muted-foreground">来源任务：{receipt.task_id}</p>
            </div>
            <Button
              variant="outline"
              disabled={!receipt.can_inspect}
              onClick={() => setSelected(receipt)}
            >
              <Search aria-hidden="true" />
              核对原订单
            </Button>
          </li>
        ))}
      </ul>
      {selected && <WorkbenchSMSInspect receipt={selected} onClose={() => setSelected(null)} />}
    </section>
  );
}
