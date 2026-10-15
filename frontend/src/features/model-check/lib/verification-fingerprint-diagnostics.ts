import {
  verificationNumber as number,
  verificationRecord as record,
  verificationRows as rows,
  verificationText as text,
  type VerificationRecord,
} from "./verification-progress";

export type FingerprintDiagnostic = {
  detail: string;
  model: string;
  repeatModel: string;
  protocol: string;
  repeatProtocol: string;
};

// Older reports already contain the original observations. Explain these facts
// without changing saved conclusions, filling missing fields, or making requests.
export function fingerprintDiagnostic(
  sample: VerificationRecord,
  result: VerificationRecord,
): FingerprintDiagnostic {
  const probes = rows(result.probes);
  const first = probes.find((probe) => probe.id === `fingerprint_${text(sample.id)}`);
  const repeat = probes.find((probe) => probe.id === `fingerprint_${text(sample.id)}_repeat`);
  const value = {
    detail: text(sample.detail),
    model: text(sample.model) || text(record(first).response_model),
    repeatModel: text(sample.repeat_model) || text(record(repeat).response_model),
    protocol: text(sample.protocol) || text(record(first).protocol),
    repeatProtocol: text(sample.repeat_protocol) || text(record(repeat).protocol),
  };
  if (value.detail) return value;
  if (!first && !value.model) value.model = "未记录";
  if (!repeat && !value.repeatModel) value.repeatModel = "未记录";
  const reasons: string[] = [];
  for (const item of [
    { label: "首次", probe: first },
    { label: "复测", probe: repeat },
  ]) {
    if (!item.probe) continue;
    if (item.probe.status === "error") {
      reasons.push(`${item.label}请求失败，请查看逐项请求错误`);
      continue;
    }
    if (!text(item.probe.response_model)) reasons.push(`${item.label}未返回型号`);
    if (!text(item.probe.protocol) || item.probe.protocol === "other")
      reasons.push(`${item.label}协议未识别`);
  }
  const firstTokens = number(sample.first),
    repeatTokens = number(sample.repeat);
  if (firstTokens !== null && repeatTokens !== null && firstTokens !== repeatTokens)
    reasons.push(`输入用量变化：${firstTokens} → ${repeatTokens} Token`);
  if (firstTokens === null) reasons.push("首次输入用量未记录");
  if (repeatTokens === null) reasons.push("复测输入用量未记录");
  if (number(sample.delta) === null)
    reasons.push("未形成基准差分，需同时具备稳定完整的本项采样与基准文本；请核对基准记录");
  if (reasons.length === 0) reasons.push("历史报告未记录逐项诊断原因，原始采样数值保留如上");
  value.detail = reasons.join("；");
  return value;
}
