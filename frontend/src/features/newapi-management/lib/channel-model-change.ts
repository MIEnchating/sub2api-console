import type { NewAPIChannel } from "@/api";

export type ChannelModelAction = "add" | "remove";
export type ChannelModelSelection = { action: ChannelModelAction; models: string[] };

export function channelModelImpact(
  channel: NewAPIChannel,
  action: ChannelModelAction,
  models: string[],
) {
  const changed = models.filter(
    (model) => channel.models.includes(model) === (action === "remove"),
  );
  return {
    channel,
    models: changed,
    remaining: channel.models.length + (action === "add" ? changed.length : -changed.length),
  };
}
