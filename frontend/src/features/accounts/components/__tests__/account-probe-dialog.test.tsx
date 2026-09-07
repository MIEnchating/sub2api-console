import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type OnboardingCandidate } from "@/api";
import { dialogContentClass } from "@/components/ui/dialog";
import { onboardingProbeTarget } from "@/App";
import {
  AccountProbeDialog,
  accountProbeDialogLayout,
  defaultProbeModelForPlatform,
  onboardingProbeModeOptions,
  onboardingProbeModelOptions,
  ProbeDialogActions,
  ProbeModelLoadButton,
  ProbeResultSlot,
  shouldLoadProbeModels,
} from "../account-probe-dialog";

afterEach(() => vi.restoreAllMocks());

describe("账号探活弹窗", () => {
  it("优先使用已选择本地分组的平台匹配平台默认探活模型", () => {
    const candidate = {
      group_id: "6",
      host: "api.example",
      group_name: "Codex 分组",
      platform: null,
    } as OnboardingCandidate;

    expect(onboardingProbeTarget(candidate, "openai")?.platform).toBe("openai");
  });

  it("按账号平台选择已配置且真实存在的默认探活模型", () => {
    const configured = {
      default: {
        models: [],
        concurrency: 10,
        load_factor: null,
        priority: 1,
        pool_mode: false,
        pool_mode_retry_count: 3,
        pool_mode_retry_status_codes: [401, 403, 429],
      },
      groups: [],
      platform_probe_models: {
        openai: "gpt-5.2",
        anthropic: "claude-sonnet-4-5",
      },
    };

    expect(defaultProbeModelForPlatform(["gpt-5.1", "gpt-5.2"], configured, "OpenAI")).toBe(
      "gpt-5.2",
    );
    expect(defaultProbeModelForPlatform(["claude-sonnet-4-5"], configured, "claude")).toBe(
      "claude-sonnet-4-5",
    );
    expect(defaultProbeModelForPlatform(["gpt-5.1"], configured, "openai")).toBe("gpt-5.1");
  });

  it("打开添加账号探活时在模型选择框显示对应平台的默认模型", async () => {
    vi.spyOn(api, "accountCreationSettings").mockResolvedValue({
      default: {
        models: [],
        concurrency: 10,
        load_factor: null,
        priority: 1,
        pool_mode: false,
        pool_mode_retry_count: 3,
        pool_mode_retry_status_codes: [401, 403, 429],
      },
      groups: [],
      platform_probe_models: { openai: "gpt-5.2" },
    });
    vi.spyOn(api, "onboardingProbeModels").mockResolvedValue({
      models: ["gpt-5.1", "gpt-5.2"],
    });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <AccountProbeDialog
          target={{
            kind: "onboarding",
            host: "api.example",
            groupId: "6",
            name: "OpenAI 账号",
            platform: "openai",
          }}
          open
          onOpenChange={() => undefined}
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("combobox", { name: "选择测试模型" })).toHaveTextContent(
      "gpt-5.2",
    );
    expect(await screen.findByRole("button", { name: "重新获取上游模型" })).toBeInTheDocument();
  });

  it("添加账号时对上游模型去重排序", () => {
    expect(onboardingProbeModelOptions(["gpt-5.2", "gpt-5.1-codex", "gpt-5.2"])).toEqual([
      "gpt-5.1-codex",
      "gpt-5.2",
    ]);
  });

  it("提供与测试连接一致的常规和 Compact 探测模式并默认常规请求", () => {
    expect(onboardingProbeModeOptions).toEqual([
      { value: "default", label: "常规请求" },
      { value: "stream", label: "Compact 探测" },
    ]);
  });

  it("点击探活打开弹窗时立即加载模型且同一目标只自动加载一次", () => {
    expect(shouldLoadProbeModels(true, null, "api.example\u00006")).toBe(true);
    expect(shouldLoadProbeModels(false, null, "api.example\u00006")).toBe(false);
    expect(shouldLoadProbeModels(true, "api.example\u00006", "api.example\u00006")).toBe(false);
    expect(shouldLoadProbeModels(true, "api.example\u00006", "api.example\u00007")).toBe(true);
  });

  it("模型获取失败时提供独立重试按钮", () => {
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

  it("测试中复用深色结果面板并预先显示模型和测试消息", () => {
    const markup = renderToStaticMarkup(
      <ProbeResultSlot pending error={null} result={null} requestModel="gpt-5.6-sol" />,
    );

    expect(markup).toContain("bg-zinc-950");
    expect(markup).toContain("使用模型：gpt-5.6-sol");
    expect(markup).toContain("发送测试消息");
    expect(markup).toContain("等待上游响应");
    expect(markup).toContain("[&amp;&gt;*]:min-h-36");
    expect(markup).not.toContain("弹窗会在测试完成后显示详细结果");
  });

  it("使用稳定宽度并按内容自适应高度，仅在超出视口时滚动", () => {
    const className = dialogContentClass(
      accountProbeDialogLayout.width,
      accountProbeDialogLayout.height,
      accountProbeDialogLayout.content,
    );

    expect(className).toContain("w-[min(32rem,calc(100vw-2rem))]");
    expect(className).toContain("grid-rows-[auto_minmax(0,1fr)_auto_auto]");
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
          message: "上游返回 HTTP 400：invalid_request_error",
          request_model: "gpt-5.2",
          actual_model: "gpt-5.2-2026-08-01",
          response_text: "Hi! How can I help?",
          latency_ms: 86,
          http_status: 200,
        }}
      />,
    );
    for (const text of ["Hi! How can I help?", "使用模型", "发送测试消息", "响应", "测试完成"]) {
      expect(markup).toContain(text);
    }
    expect(markup).toContain("[overflow-wrap:anywhere]");
    expect(markup).toContain("min-h-36");

    const failedMarkup = renderToStaticMarkup(
      <ProbeResultSlot
        pending={false}
        error={null}
        result={{
          status: "failed",
          message: "上游返回 HTTP 400：invalid_request_error",
          request_model: "gpt-5.2",
          actual_model: "",
          latency_ms: 86,
          http_status: 400,
        }}
      />,
    );
    expect(failedMarkup).toContain("invalid_request_error");
    expect(failedMarkup).toContain("测试失败");
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
