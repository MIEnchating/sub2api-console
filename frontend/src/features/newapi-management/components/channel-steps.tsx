import { Check } from "lucide-react";
import { CardAction, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

export function NewAPIChannelSteps(props: { configurationReady: boolean }) {
  return (
    <CardHeader>
      <CardTitle>
        <h2>添加 Sub2API 渠道</h2>
      </CardTitle>
      <CardDescription>步骤 {props.configurationReady ? "2" : "1"} / 2</CardDescription>
      <CardAction>
        <ol className="flex min-w-0 flex-wrap items-center gap-3" aria-label="添加渠道步骤">
          <li
            data-channel-step="credentials"
            data-state={props.configurationReady ? "complete" : "current"}
            aria-current={props.configurationReady ? undefined : "step"}
            className={cn(
              "flex min-w-0 items-center gap-2 text-xs font-medium",
              props.configurationReady ? "text-muted-foreground" : "text-foreground",
            )}
          >
            <span
              aria-hidden="true"
              className={cn(
                "flex size-6 shrink-0 items-center justify-center rounded-full border tabular-nums",
                props.configurationReady
                  ? "border-primary/30 bg-primary/10 text-primary"
                  : "border-primary bg-primary text-primary-foreground",
              )}
            >
              {props.configurationReady ? <Check className="size-3.5" /> : 1}
            </span>
            <span>创建密钥</span>
          </li>
          <li
            className={cn(
              "h-px w-6 shrink-0",
              props.configurationReady ? "bg-primary/40" : "bg-border",
            )}
            aria-hidden="true"
          />
          <li
            data-channel-step="configuration"
            data-state={props.configurationReady ? "current" : "upcoming"}
            aria-current={props.configurationReady ? "step" : undefined}
            className={cn(
              "flex min-w-0 items-center gap-2 text-xs font-medium",
              props.configurationReady ? "text-foreground" : "text-muted-foreground",
            )}
          >
            <span
              aria-hidden="true"
              className={cn(
                "flex size-6 shrink-0 items-center justify-center rounded-full border tabular-nums",
                props.configurationReady
                  ? "border-primary bg-primary text-primary-foreground"
                  : "bg-muted/30",
              )}
            >
              2
            </span>
            <span>配置渠道</span>
          </li>
        </ol>
      </CardAction>
    </CardHeader>
  );
}
