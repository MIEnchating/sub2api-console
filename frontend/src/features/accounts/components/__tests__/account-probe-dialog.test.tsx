import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { dialogContentClass } from "@/components/ui/dialog";
import {
  accountProbeDialogContentClass,
  onboardingProbeModelOptions,
  ProbeDialogActions,
  ProbeResultPanel,
} from "../account-probe-dialog";

describe("账号探活弹窗", () => {
  it("添加账号时对上游模型去重排序", () => {
    expect(onboardingProbeModelOptions(["gpt-5.2", "gpt-5.1-codex", "gpt-5.2"])).toEqual([
      "gpt-5.1-codex",
      "gpt-5.2",
    ]);
  });

  it("使用稳定宽度并限制所有探活控件在弹窗内部", () => {
    const className = dialogContentClass("medium", "content", accountProbeDialogContentClass);

    expect(className).toContain("w-[min(32rem,calc(100vw-2rem))]");
    expect(className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(className).toContain("min-w-0");
    expect(className).toContain("overflow-hidden");
  });

  it("在弹窗内容中显示探活结果明细", () => {
    const markup = renderToStaticMarkup(
      <ProbeResultPanel
        result={{
          status: "passed",
          message: `上游返回 HTTP 400：${"invalid_request_error".repeat(20)}`,
          request_model: "gpt-5.2",
          actual_model: "gpt-5.2-2026-08-01",
          latency_ms: 86,
          http_status: 200,
        }}
      />,
    );
    for (const text of ["探活通过", "invalid_request_error", "gpt-5.2", "HTTP 状态", "86 毫秒"]) {
      expect(markup).toContain(text);
    }
    expect(markup).toContain("[overflow-wrap:anywhere]");
  });

  it("探活进行中允许关闭弹窗并阻止重复测试", () => {
    const markup = renderToStaticMarkup(
      <ProbeDialogActions
        runDisabled
        probePending
        hasResult={false}
        onClose={() => undefined}
        onRun={() => undefined}
      />,
    );
    const closeButton = markup.match(/<button[^>]*>关闭<\/button>/)?.[0];
    const probeButton = markup.match(/<button[^>]*disabled=""[^>]*>.*测试中<\/button>/)?.[0];

    expect(closeButton).toBeDefined();
    expect(closeButton).not.toContain(' disabled=""');
    expect(probeButton).toBeDefined();
  });
});
