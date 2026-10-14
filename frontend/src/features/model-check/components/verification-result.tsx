import { useState, type ReactElement } from "react";
import { Badge } from "@/components/ui/badge";
import {
  verificationCapabilityLabels as capabilityLabels,
  verificationProbeLabels as probeLabels,
  verificationProtocolLabels,
} from "../constants";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

type RecordValue = Record<string, unknown>;

function record(value: unknown): RecordValue {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as RecordValue)
    : {};
}

function rows(value: unknown): RecordValue[] {
  return Array.isArray(value) ? value.map(record) : [];
}

function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function numberText(value: unknown): string {
  return typeof value === "number" && Number.isFinite(value) ? String(value) : "—";
}

export function VerificationResult(props: { result: RecordValue }): ReactElement {
  const [open, setOpen] = useState(false);
  const authenticity = record(props.result.authenticity);
  const capability = record(props.result.capability);
  const status = text(capability.status);
  let variant: "success" | "destructive" | "warning" = "warning";
  if (status === "passed") variant = "success";
  if (status === "failed") variant = "destructive";
  return (
    <div className="grid gap-2">
      <div className="flex flex-wrap gap-1.5">
        <Badge variant="warning">身份未证实</Badge>
        <Badge variant={variant}>{capabilityLabels[status] ?? capabilityLabels.inconclusive}</Badge>
      </div>
      <p className="text-muted-foreground text-xs whitespace-normal">{text(capability.summary)}</p>
      <Button type="button" variant="outline" onClick={() => setOpen(true)}>
        查看检测证据
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent width="table" height="tall">
          <DialogHeader>
            <DialogTitle>检测证据</DialogTitle>
          </DialogHeader>
          <DialogBody className="space-y-4">
            <section aria-label="真实性与能力结论" className="space-y-2">
              <p className="text-sm">
                已执行 {rows(props.result.probes).length} / {numberText(props.result.total)} 项 ·
                通过 {numberText(props.result.passed)} 项
              </p>
              <p className="text-sm">{text(authenticity.summary)}</p>
              <p className="text-sm">{text(capability.summary)}</p>
            </section>
            <section aria-label="真实性证据" className="grid gap-2">
              {rows(props.result.evidence).map((item, index) => (
                <div key={text(item.id) || index} className="rounded-md border p-3">
                  <p className="font-medium">{text(item.label)}</p>
                  <p className="text-muted-foreground mt-1 text-sm wrap-anywhere">
                    {text(item.detail)}
                  </p>
                </div>
              ))}
            </section>
            <section aria-label="逐项检测结果" className="grid gap-3">
              {rows(props.result.probes).map((item, index) => (
                <article
                  key={text(item.request_id) || index}
                  className="space-y-2 rounded-md border p-3"
                >
                  <p className="font-medium">
                    {text(item.label)} · {probeLabels[text(item.status)] ?? "待判定"}
                  </p>
                  <p className="text-muted-foreground text-xs wrap-anywhere">
                    第 {numberText(item.round)} 轮 ·{" "}
                    {verificationProtocolLabels[text(item.protocol)] ?? "协议未记录"} · HTTP{" "}
                    {numberText(item.http_status)} · {numberText(item.duration_ms)} ms
                  </p>
                  <p className="text-muted-foreground text-xs wrap-anywhere">
                    返回型号：{text(item.response_model) || "未返回"} · 输入{" "}
                    {numberText(record(item.usage).input_tokens)} / 输出{" "}
                    {numberText(record(item.usage).output_tokens)} Token
                  </p>
                  {item.answer_truncated === true ? (
                    <p className="text-xs">回答过长，仅展示截取内容；判定使用完整回答。</p>
                  ) : null}
                  <p className="text-muted-foreground text-xs wrap-anywhere">
                    请求 ID：{text(item.request_id)}
                  </p>
                  <details>
                    <summary className="cursor-pointer text-sm">查看题目</summary>
                    <p className="mt-2 whitespace-pre-wrap wrap-anywhere text-sm">
                      {text(item.prompt)}
                    </p>
                  </details>
                  <p className="text-sm wrap-anywhere">预期：{text(item.expected)}</p>
                  <pre className="bg-muted max-h-64 overflow-auto rounded p-2 text-xs whitespace-pre-wrap wrap-anywhere">
                    {text(item.answer) || text(item.error) || "未返回回答"}
                  </pre>
                </article>
              ))}
            </section>
          </DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  );
}
