import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { onboardingProbeModelOptions, ProbeResultPanel } from "../account-probe-dialog";

describe("账号探活弹窗", () => {
  it("添加账号时对上游模型去重排序", () => {
    expect(onboardingProbeModelOptions(["gpt-5.2", "gpt-5.1-codex", "gpt-5.2"])).toEqual([
      "gpt-5.1-codex",
      "gpt-5.2",
    ]);
  });

  it("在弹窗内容中显示探活结果明细", () => {
    const markup = renderToStaticMarkup(
      <ProbeResultPanel
        result={{
          status: "passed",
          message: "上游已返回有效响应",
          request_model: "gpt-5.2",
          actual_model: "gpt-5.2-2026-08-01",
          latency_ms: 86,
          http_status: 200,
        }}
      />,
    );
    for (const text of ["探活通过", "上游已返回有效响应", "gpt-5.2", "HTTP 状态", "86 毫秒"]) {
      expect(markup).toContain(text);
    }
  });
});
