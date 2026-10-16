import { VerificationPublicReference } from "./verification-public-reference";
import { VerificationBehaviorDetails } from "./verification-behavior-details";
import type { ReactElement } from "react";
import { VerificationFingerprintDiagnostics } from "./verification-fingerprint-diagnostics";
import { VerificationStateBadge } from "./verification-state";
import {
  verificationFingerprintLabels,
  verificationFingerprintSampleLabels,
  verificationReferenceLabels,
} from "../constants";
import {
  verificationRows as rows,
  verificationRecord as record,
  verificationNumber as number,
  verificationText as text,
  type VerificationRecord,
} from "../lib/verification-progress";

export function VerificationFingerprintDetails(props: {
  result: VerificationRecord;
}): ReactElement {
  const fingerprint = record(props.result.fingerprint);
  const samples = rows(fingerprint.samples);
  const comparisons = rows(props.result.reference_comparisons);
  return (
    <>
      <VerificationPublicReference result={props.result} />
      <VerificationBehaviorDetails result={props.result} />
      {fingerprint.status ? (
        <section aria-label="分词指纹采样" className="space-y-2 rounded-md border p-3">
          <h3 className="font-medium">
            分词指纹采样 · {verificationFingerprintLabels[text(fingerprint.status)] ?? "待判定"}
          </h3>
          <p className="text-xs text-muted-foreground">{text(fingerprint.summary)}</p>
          {samples.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full min-w-72 whitespace-nowrap text-left text-xs">
                <thead>
                  <tr>
                    <th className="p-2">文本</th>
                    <th>首次输入</th>
                    <th>复测输入</th>
                    <th>减去基准</th>
                    <th>状态</th>
                  </tr>
                </thead>
                <tbody>
                  {samples.map((sample) => (
                    <tr key={text(sample.id)} className="border-t">
                      <th className="p-2 font-normal">{text(sample.label)}</th>
                      <td>{number(sample.first) ?? "—"}</td>
                      <td>{number(sample.repeat) ?? "—"}</td>
                      <td>{number(sample.delta) ?? "—"}</td>
                      <td>
                        <VerificationStateBadge
                          status={text(sample.status)}
                          label={
                            verificationFingerprintSampleLabels[text(sample.status)] ?? "待判定"
                          }
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
          {samples.length > 0 ? (
            <VerificationFingerprintDiagnostics samples={samples} result={props.result} />
          ) : null}
        </section>
      ) : null}
      {comparisons.length > 0 ? (
        <section aria-label="官方指纹对照明细" className="space-y-3">
          {comparisons.map((comparison, index) => (
            <article
              key={`${text(comparison.account_id)}-${index}`}
              className="space-y-2 rounded-md border p-3"
            >
              <h3 className="font-medium">
                参考账号 {text(comparison.account_id)} ·{" "}
                {verificationReferenceLabels[text(comparison.status)] ?? "待判定"}
              </h3>
              <p className="text-xs text-muted-foreground">{text(comparison.summary)}</p>
              {rows(comparison.samples).length > 0 ? (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-72 whitespace-nowrap text-left text-xs">
                    <thead>
                      <tr>
                        <th className="p-2">文本</th>
                        <th>当前差分</th>
                        <th>参考差分</th>
                        <th>两侧差值</th>
                      </tr>
                    </thead>
                    <tbody>
                      {rows(comparison.samples).map((sample) => (
                        <tr key={text(sample.label)} className="border-t">
                          <th className="p-2 font-normal">{text(sample.label)}</th>
                          <td>{number(sample.observed) ?? "—"}</td>
                          <td>{number(sample.reference) ?? "—"}</td>
                          <td>{number(sample.difference) ?? "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : null}
            </article>
          ))}
        </section>
      ) : null}
    </>
  );
}
