import { api, type NewAPIChannel } from "@/api";

const discoveryConcurrency = 4;

export async function readCommonChannelModels(
  platformId: string,
  channels: readonly NewAPIChannel[],
  signal: AbortSignal,
): Promise<string[]> {
  const failed = new AbortController();
  const requestSignal = AbortSignal.any([signal, failed.signal]);
  let common: string[] | undefined;
  try {
    // Retain at most one batch of model lists; all channels must validate before
    // publishing a result, even when an earlier batch has no common models.
    for (let start = 0; start < channels.length; start += discoveryConcurrency) {
      requestSignal.throwIfAborted();
      const results = await Promise.all(
        channels
          .slice(start, start + discoveryConcurrency)
          .map((channel) => api.availableNewAPIChannelModels(platformId, channel, requestSignal)),
      );
      requestSignal.throwIfAborted();
      for (const result of results) {
        const supported = new Set(result.models);
        common = common ? common.filter((model) => supported.has(model)) : result.models;
      }
    }
    return common ?? [];
  } catch (error) {
    failed.abort();
    throw error;
  }
}
