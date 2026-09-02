import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { dialogContentClass } from "@/components/ui/dialog";
import {
  accountProbeDialogLayout,
  onboardingProbeModelOptions,
  ProbeDialogActions,
  ProbeModelLoadButton,
  ProbeResultSlot,
  shouldLoadProbeModels,
} from "../account-probe-dialog";

describe("账号探活弹窗", () => {
  it("添加账号时对上游模型去重排序", () => {
    expect(onboardingProbeModelOptions(["gpt-5.2", "gpt-5.1-codex", "gpt-5.2"])).toEqual([
      "gpt-5.1-codex",
      "gpt-5.2",
    ]);
  });

  it("点击探活打开弹窗时立即加载模型且同一目标只自动加载一次", () => {
    expect(shouldLoadProbeModels(true, null, "api.example\u00006")).toBe(true);
    expect(shouldLoadProbeModels(false, null, "api.example\u00006")).toBe(false);
    expect(shouldLoadProbeModels(true, "api.example\u00006", "api.example\u00006")).toBe(false);
    expect(shouldLoadProbeModels(true, "api.example\u00006", "api.example\u00007")).toBe(true);
  });

  it("保留独立的上游模型获取按钮", () => {
    const markup = renderToStaticMarkup(
      <ProbeModelLoadButton
        pending={false}
        succeeded={false}
        disabled={false}
        onLoad={() => undefined}
      />,
    );

    expect(markup).toContain("获取上游模型");
    expect(markup).toContain('type="button"');
    expect(markup).not.toContain('disabled=""');
  });

  it("模型获取完成后按钮退出加载状态", () => {
    const markup = renderToStaticMarkup(
      <ProbeModelLoadButton pending={false} succeeded disabled={false} onLoad={() => undefined} />,
    );

    expect(markup).toContain("重新获取上游模型");
    expect(markup).not.toContain("正在获取");
    expect(markup).not.toContain("animate-spin");
  });

  it("使用稳定宽度并按内容自适应高度，仅在超出视口时滚动", () => {
    const className = dialogContentClass(
      accountProbeDialogLayout.width,
      accountProbeDialogLayout.height,
      accountProbeDialogLayout.content,
    );

    expect(className).toContain("w-[min(32rem,calc(100vw-2rem))]");
    expect(className).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(className).toContain("max-h-[calc(100svh-2rem)]");
    expect(className).not.toContain("h-[min(42rem");
    expect(className).toContain("overflow-hidden");
  });

  it("在弹窗内容中显示探活结果明细", () => {
    const markup = renderToStaticMarkup(
      <ProbeResultSlot
        pending={false}
        error={null}
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
    expect(markup).toContain("min-h-36");
  });

  it("尚未探活时不显示底部结果占位", () => {
    const markup = renderToStaticMarkup(
      <ProbeResultSlot pending={false} error={null} result={null} />,
    );

    expect(markup).toBe("");
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
