import type { NewAPIChannel } from "@/api";
import { channelModelImpact, type ChannelModelAction } from "../lib/channel-model-change";

export function ChannelImpactPreview(props: {
  channels: NewAPIChannel[];
  action: ChannelModelAction;
  models: string[];
}) {
  return (
    <div className="grid gap-3">
      <p className="text-sm">
        将对 {props.channels.length} 个渠道执行模型{props.action === "add" ? "上架" : "下架"}
        ，没有变化的渠道会跳过写入。
      </p>
      <ul
        aria-label="变更模型"
        className="flex max-h-36 flex-wrap gap-2 overflow-y-auto rounded-md border bg-muted/20 p-3"
      >
        {props.models.map((model) => (
          <li
            key={model}
            className="max-w-full break-all rounded border bg-background px-2 py-1 font-mono text-xs"
          >
            {model}
          </li>
        ))}
      </ul>
      <div
        aria-label="渠道影响范围"
        role="group"
        className="max-h-64 overflow-y-auto rounded-md border"
      >
        {props.channels.map((channel) => {
          const impact = channelModelImpact(channel, props.action, props.models);
          return (
            <div
              key={channel.id}
              className="flex flex-wrap items-center justify-between gap-2 border-b p-3 last:border-b-0"
            >
              <div className="min-w-0">
                <p className="break-all text-sm font-medium">{channel.name}</p>
                <p className="text-muted-foreground text-xs">ID {channel.id}</p>
              </div>
              <p className="text-muted-foreground text-xs">
                变更 {impact.models.length} 个 · {channel.models.length} → {impact.remaining} 个模型
              </p>
            </div>
          );
        })}
      </div>
    </div>
  );
}
