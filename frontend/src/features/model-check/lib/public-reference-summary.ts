import {
  verificationNumber as number,
  verificationRecord as record,
  verificationRows as rows,
  verificationText as text,
  type VerificationRecord,
} from "./verification-progress";

// Older reports have only a generic summary. Use their saved failed requests
// to explain missing evidence without changing or recomputing the verdict.
export function publicReferenceSummary(result: VerificationRecord): string {
  const reference = record(result.public_reference);
  const knowledge = record(reference.knowledge);
  const summary = text(reference.summary);
  if (knowledge.status !== "inconclusive") return summary;
  const failures = rows(result.probes).filter(
    (probe) => text(probe.id).startsWith("public_kbf_") && probe.status === "error",
  );
  const first = failures[0];
  if (!first) return summary;
  const reason = `${text(first.label) || text(first.id)}：${text(first.error) || "请求失败，未取得有效回答"}`;
  if (Array.isArray(knowledge.reasons) && knowledge.reasons.length > 0)
    return `${summary} 失败项：${reason}`;
  return `${failures.length} 个知识边界请求失败，无法判定；已保留有效答案 ${number(knowledge.valid) ?? 0}/${number(knowledge.total) ?? 0} 题。失败项：${reason}。请检查接口后重试。`;
}
