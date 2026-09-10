import type { KumaMonitor } from "@/api";
import { ResultSummaryRow } from "@/components/result-summary-row";
import { StatusBadge, type StatusVariant } from "@/components/status-badge";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { monitorStatus, monitorStatusVariants, monitorTypeLabels } from "../constants";
import { PushURL } from "./push-url";

export function MonitorStatusBadge(props: { monitor: KumaMonitor; management: boolean }) {
  const label = monitorStatus(props.monitor, props.management);
  const variants: Record<string, StatusVariant> = monitorStatusVariants;
  return <StatusBadge label={label} variant={variants[label] ?? "neutral"} />;
}

export function MonitorDetail(props: {
  monitor: KumaMonitor;
  management: boolean;
  onClose: () => void;
}) {
  const monitor = props.monitor;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent width="medium">
        <DialogHeader>
          <DialogTitle className="wrap-anywhere">{monitor.name}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <MonitorStatusBadge monitor={monitor} management={props.management} />
          {monitor.type === "push" && props.management && (
            <PushURL id={monitor.id} revision={monitor.revision} />
          )}
          <div className="divide-border/70 divide-y">
            <ResultSummaryRow
              label="监控类型"
              value={monitorTypeLabels[monitor.type] ?? monitor.type}
            />
            {monitor.id > 0 && <ResultSummaryRow label="监控 ID" value={String(monitor.id)} />}
            {(monitor.target || monitor.url) && (
              <ResultSummaryRow label="监控地址" value={monitor.target || monitor.url} />
            )}
            <ResultSummaryRow
              label="当前响应"
              value={
                monitor.response_time === null
                  ? "暂无数据"
                  : `${monitor.response_time.toFixed(0)} ms`
              }
            />
            <ResultSummaryRow
              label="在线时间（24 小时）"
              value={monitor.uptime === null ? "暂无数据" : `${(monitor.uptime * 100).toFixed(2)}%`}
            />
            <ResultSummaryRow
              label="证书剩余有效期"
              value={
                monitor.certificate_days === null
                  ? "暂无数据"
                  : `${monitor.certificate_days.toFixed(0)} 天`
              }
            />
            <ResultSummaryRow
              label="检测间隔"
              value={monitor.interval > 0 ? `${monitor.interval} 秒` : "暂无数据"}
            />
          </div>
          {monitor.url_redacted && (
            <p className="px-3 py-2 text-xs text-muted-foreground">地址中的凭据和查询参数已隐藏</p>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
