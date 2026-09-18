const legacyCombinedReasons = new Set(["上游限流或额度不足", "限流或额度不足"]);

export function recentFailureReason(reason: string | null): string | null {
  const text = reason?.trim();
  if (!text) return null;
  if (legacyCombinedReasons.has(text)) return "历史记录未保存具体错误，请查看后续探活结果";
  return text;
}

/** 仅按明确错误信息细化展示；HTTP 429、quota 等合并信号不能证明余额不足。 */
export function recentLimitFailureLabel(reason: string | null): string | null {
  const text = recentFailureReason(reason) ?? "";
  const balance =
    /\binsufficient[ _-](?:balance|credits?|funds)\b|\b(?:balance|credits?) (?:is )?(?:insufficient|exhausted)\b|余额不足|余额耗尽|余额已用完|欠费/i.test(
      text,
    );
  const rateLimit =
    /\brate[ _-]limit(?:ed|[ _-](?:exceeded|error))?\b|\btoo many requests\b|限流|请求过于频繁/i.test(
      text,
    );
  if (balance && !rateLimit) return "余额不足";
  if (rateLimit && !balance) return "限流";
  return null;
}
