import type { ReactElement } from "react";
import { verificationProtocolLabels } from "../constants";
import { fingerprintDiagnostic } from "../lib/verification-fingerprint-diagnostics";
import { verificationText as text, type VerificationRecord } from "../lib/verification-progress";

export function VerificationFingerprintDiagnostics(props: {
  samples: VerificationRecord[];
  result: VerificationRecord;
}): ReactElement {
  return (
    <div className="space-y-3 border-t pt-3" aria-label="采样依据">
      {props.samples.map((sample) => {
        const value = fingerprintDiagnostic(sample, props.result);
        return (
          <div key={text(sample.id)} className="min-w-0 space-y-1 text-xs wrap-anywhere">
            <h4 className="font-medium">{text(sample.label)} · 采样依据</h4>
            <p>{value.detail}</p>
            <p className="text-muted-foreground">
              首次型号：{value.model || "未返回"} · 复测型号：{value.repeatModel || "未返回"}
            </p>
            <p className="text-muted-foreground">
              首次协议：{verificationProtocolLabels[value.protocol] ?? "未识别"} · 复测协议：
              {verificationProtocolLabels[value.repeatProtocol] ?? "未识别"}
            </p>
          </div>
        );
      })}
    </div>
  );
}
