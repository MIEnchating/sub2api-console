import type { ReactElement } from "react";

export function VerificationSetup(): ReactElement {
  return (
    <section
      aria-label="检测步骤"
      className="shrink-0 space-y-1 rounded-md border bg-muted/20 p-3 text-sm"
    >
      <p className="font-medium">模型检测：能力检测、指纹采样与真伪分析一次完成</p>
      <p className="text-xs text-muted-foreground">
        根据所选模型自动匹配公开参考，统一展示结论和依据。没有可用参考时说明原因，不用相近型号代替。
      </p>
    </section>
  );
}
