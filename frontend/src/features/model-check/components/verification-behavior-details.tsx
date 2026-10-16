import type { ReactElement } from "react";
import {
  verificationRecord as record,
  verificationRows as rows,
  verificationNumber as number,
  verificationText as text,
  type VerificationRecord,
} from "../lib/verification-progress";
import { verificationBehaviorLabels } from "../constants";

export function VerificationBehaviorDetails(props: {
  result: VerificationRecord;
}): ReactElement | null {
  const fingerprint = record(props.result.behavior_fingerprint);
  if (!fingerprint.version) return null;
  return (
    <section aria-label="行为指纹采样" className="min-w-0 space-y-2 rounded-md border p-3">
      <h3 className="font-medium">
        行为指纹 · {verificationBehaviorLabels[text(fingerprint.status)] ?? "无法判定"}
      </h3>
      <p className="text-sm">{text(fingerprint.summary)}</p>
      <p className="text-xs text-muted-foreground">{text(fingerprint.sampling)}</p>
      <p className="text-xs text-muted-foreground">
        有效样本 {number(fingerprint.valid) ?? 0}/{number(fingerprint.planned) ?? 200} ·{" "}
        {text(fingerprint.version)} · {text(fingerprint.profile)}
      </p>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[34rem] text-left text-xs">
          <thead>
            <tr>
              <th className="p-2">采样单元</th>
              <th>有效</th>
              <th>无效</th>
              <th>请求失败</th>
              <th>未执行</th>
              <th>奇偶 JSD</th>
              <th>回答分布</th>
            </tr>
          </thead>
          <tbody>
            {rows(fingerprint.cells).map((cell) => (
              <tr key={text(cell.id)} className="border-t">
                <th className="p-2 font-normal">
                  {text(cell.label)}
                  <span className="block text-muted-foreground">
                    {verificationBehaviorLabels[text(cell.status)] ?? "无法判定"}
                  </span>
                </th>
                <td>{number(cell.valid) ?? 0}</td>
                <td>{number(cell.invalid) ?? 0}</td>
                <td>{number(cell.errors) ?? 0}</td>
                <td>{number(cell.missing) ?? 0}</td>
                <td>{number(cell.self_jsd)?.toFixed(3) ?? "—"}</td>
                <td className="max-w-56 break-words">
                  {Object.entries(record(cell.counts))
                    .map(([answer, count]) => `${answer}: ${number(count) ?? 0}`)
                    .join("、") || "无有效回答"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-muted-foreground">
        JSD 范围 0～1，越大表示本次奇偶回答分布差异越大；0
        也不能证明模型相同。无效格式与拒答不进入分布，缺失不补零。身份结论：无法判定。
      </p>
    </section>
  );
}
