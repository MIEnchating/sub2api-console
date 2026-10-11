import type { NewAPIChannel } from "@/api";

export const automationChannelLimit = 50;

type ChannelTarget = { id: string; version: string };

export type AutomationChannelSelection = {
  options: NewAPIChannel[];
  selected: ReadonlySet<string>;
  unavailableIDs: string[];
  targets: ChannelTarget[] | null;
  signature: string;
};

export function resolveAutomationChannels(
  channels: NewAPIChannel[],
  selectedIDs: string[],
): AutomationChannelSelection {
  const catalog = new Map<string, NewAPIChannel | null>();
  for (const channel of channels) {
    // A changing upstream page can repeat a channel. Do not choose a version
    // arbitrarily or render multiple controls for the same stable ID.
    catalog.set(channel.id, catalog.has(channel.id) ? null : channel);
  }
  const targets: ChannelTarget[] = [];
  const unavailableIDs: string[] = [];
  const signature: Array<[string, string | null]> = [];
  for (const id of selectedIDs) {
    const channel = catalog.get(id);
    signature.push([id, channel?.version ?? null]);
    if (channel) targets.push({ id, version: channel.version });
    else unavailableIDs.push(id);
  }
  const selected = new Set(selectedIDs);
  const valid =
    unavailableIDs.length === 0 &&
    selectedIDs.length <= automationChannelLimit &&
    selected.size === selectedIDs.length;
  return {
    options: [...catalog.values()].filter((channel): channel is NewAPIChannel => channel !== null),
    selected,
    unavailableIDs,
    targets: valid ? targets : null,
    signature: JSON.stringify(signature),
  };
}
