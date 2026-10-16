import type { ReactElement } from "react";
import { publicReferenceSummary } from "../lib/public-reference-summary";
import { publicReferenceLabels, publicReferenceEvidenceLabels } from "../constants";
import {
  verificationRecord as record,
  verificationRows as rows,
  verificationNumber as number,
  verificationText as text,
  type VerificationRecord,
} from "../lib/verification-progress";
import { VerificationStateBadge } from "./verification-state";

export function VerificationPublicReference(props: {
  result: VerificationRecord;
}): ReactElement | null {
  const reference = record(props.result.public_reference);
  if (!reference.version) return null;
  const knowledge = record(reference.knowledge);
  const tokenizer = record(reference.tokenizer);
  const distribution = record(reference.distribution);
  return (
    <section aria-label="公开模型指纹对比" className="min-w-0 space-y-3 rounded-md border p-3">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="font-medium">公开模型指纹对比</h3>
        <VerificationStateBadge
          status={text(reference.status)}
          label={
            publicReferenceLabels[text(reference.status)] ?? publicReferenceLabels.inconclusive
          }
        />
      </div>
      <p className="text-sm wrap-anywhere">{publicReferenceSummary(props.result)}</p>
      <p className="text-xs text-muted-foreground wrap-anywhere">
        参考型号：{text(reference.model)} · {text(reference.version)}
      </p>
      {knowledge.status ? (
        <div className="space-y-1 text-sm">
          <h4 className="font-medium">知识边界比较</h4>
          <p>
            有效 {number(knowledge.valid) ?? 0}/{number(knowledge.total) ?? 0} 题 · 差异{" "}
            {number(knowledge.mismatches) ?? 0} 题
          </p>
          <p>
            参考自测差异 {number(knowledge.self_mismatches) ?? 0}/
            {number(knowledge.self_total) ?? 0} · 单侧检验 p 值：
            {number(knowledge.p_value)?.toPrecision(4) ?? "未计算"}
          </p>
          <p className="text-xs text-muted-foreground">
            参考提供商（来源自述）：{text(knowledge.provider)} · {text(knowledge.date)}。p
            值不是模型为真的概率；检验采用参考自测误差的 99% 上界和 0.05
            差异阈值。缺失、参数不兼容及中断不判假。
          </p>
        </div>
      ) : null}
      {tokenizer.status ? (
        <div className="space-y-1">
          <h4 className="font-medium">
            公开分词对照 · {publicReferenceEvidenceLabels[text(tokenizer.status)] ?? "无法判定"}
          </h4>
          <p className="text-xs text-muted-foreground">
            {text(tokenizer.provider)} · {text(tokenizer.date)} · 题目版本{" "}
            {text(tokenizer.probe_set)}。差分相同可能仅代表共用分词器；计量改写也会造成差异。
          </p>
          <div className="overflow-x-auto">
            <table className="w-full min-w-72 text-left text-xs">
              <thead>
                <tr>
                  <th className="p-2">样本</th>
                  <th>参考差分</th>
                  <th>本次差分</th>
                  <th>证据状态</th>
                </tr>
              </thead>
              <tbody>
                {rows(tokenizer.samples).map((sample) => (
                  <tr key={text(sample.id)} className="border-t">
                    <th className="p-2 font-normal">{text(sample.id)}</th>
                    <td>{number(sample.reference) ?? "—"}</td>
                    <td>{number(sample.observed) ?? "—"}</td>
                    <td>{publicReferenceEvidenceLabels[text(sample.status)] ?? "无法判定"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
      {distribution.status ? (
        <div className="space-y-1">
          <h4 className="font-medium">
            回答分布比较 · 平均 JSD {number(distribution.mean_jsd)?.toFixed(3) ?? "未计算"}
          </h4>
          <p className="text-xs text-muted-foreground">
            温度 1，8 组中英文数字题各 30 次。JSD 为 0～1
            的分布差异，不是身份置信度。公开分布没有本题目子集的真伪阈值；拒答与格式异常单列，不补零。
          </p>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[32rem] text-left text-xs">
              <thead>
                <tr>
                  <th className="p-2">采样单元</th>
                  <th>有效 / 参考</th>
                  <th>无效</th>
                  <th>失败 / 缺失</th>
                  <th>JSD</th>
                </tr>
              </thead>
              <tbody>
                {rows(distribution.cells).map((cell) => (
                  <tr key={text(cell.id)} className="border-t">
                    <th className="p-2 font-normal">{text(cell.id)}</th>
                    <td>
                      {number(cell.valid) ?? 0} / {number(cell.reference_valid) ?? 0}
                    </td>
                    <td>{number(cell.invalid) ?? 0}</td>
                    <td>
                      {number(cell.errors) ?? 0} / {number(cell.missing) ?? 0}
                    </td>
                    <td>{number(cell.jsd)?.toFixed(3) ?? "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
      <details className="text-xs text-muted-foreground">
        <summary className="cursor-pointer">参考来源与校验值</summary>
        <p className="mt-2 wrap-anywhere">本地参考库 SHA-256：{text(reference.digest)}</p>
        {rows(reference.sources).map((source) => {
          const url = text(source.url);
          const safe =
            /^https:\/\/github\.com\/(Ooo0ption\/KBF|unclecode\/modelprint|dreamor\/llm-fingerprint)\/blob\//.test(
              url,
            );
          return (
            <p key={url} className="mt-2 wrap-anywhere">
              {safe ? (
                <a href={url} target="_blank" rel="noreferrer" className="underline">
                  {text(source.name)}
                </a>
              ) : (
                text(source.name)
              )}{" "}
              · {text(source.license)}
              <br />
              版本 {text(source.revision)}
              <br />
              原文件 SHA-256：{text(source.sha256)}
            </p>
          );
        })}
      </details>
    </section>
  );
}
