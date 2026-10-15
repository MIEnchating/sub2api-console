import { ArrowRight } from "lucide-react";
import type { ReactElement } from "react";
import { Badge } from "@/components/ui/badge";
import type { OnboardingBindingPreview } from "./onboarding-confirm-dialog";

function schedulingLabel(item: OnboardingBindingPreview): string {
  if (item.status === "待更新") return "保持原值";
  if (item.waitingForCapacity) return "停用等待";
  return "开启";
}

export function OnboardingAccountSummary(props: {
  item: OnboardingBindingPreview;
  index: number;
}): ReactElement {
  const item = props.item;
  const fields = [
    { label: "账号协议", value: item.platform },
    { label: "账号成本", value: item.multiplier },
    { label: "并发", value: item.status === "待更新" ? "保持原值" : item.concurrency },
    { label: "优先级", value: item.status === "待更新" ? "保持原值" : item.priority },
    { label: "调度状态", value: schedulingLabel(item) },
  ];
  return (
    <div className="bg-muted/30 grid min-w-0 gap-3 p-3 sm:px-4">
      <div className="flex min-w-0 items-start gap-2.5">
        <span
          aria-hidden="true"
          className="bg-background text-muted-foreground flex size-6 shrink-0 items-center justify-center rounded-md border text-xs tabular-nums"
        >
          {props.index + 1}
        </span>
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1 pt-0.5 font-medium">
          <span className="min-w-0 [overflow-wrap:anywhere]">{item.upstreamGroup}</span>
          <ArrowRight aria-hidden="true" className="text-muted-foreground size-3.5 shrink-0" />
          <span className="min-w-0 [overflow-wrap:anywhere]">{item.localGroup}</span>
        </div>
        <Badge
          variant={item.status === "待添加" ? "secondary" : "outline"}
          className="mt-0.5 shrink-0"
        >
          {item.status}
        </Badge>
      </div>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-xs tabular-nums sm:grid-cols-5">
        {fields.map((field) => (
          <div key={field.label} className="grid min-w-0 content-start gap-1">
            <dt className="text-muted-foreground">{field.label}</dt>
            <dd className="font-medium [overflow-wrap:anywhere]">{field.value}</dd>
          </div>
        ))}
      </dl>
      {item.status === "待添加" && item.allocationOverride !== undefined ? (
        <p className="text-muted-foreground border-t pt-2 text-xs">
          共享并发分配：{item.allocationOverride ? "单独开启" : "单独关闭"}
          （总开关与容量保护继续生效）
        </p>
      ) : null}
      {item.waitingForCapacity ? (
        <p className="text-muted-foreground border-t pt-2 text-xs">
          <span className="text-foreground font-medium">等待并发额度</span>：账号将以并发 1
          创建并保持停用，启用前须由调度器重新核对容量。
        </p>
      ) : null}
    </div>
  );
}
