import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { AnimationResult } from "@/api";
import { AnimationAccountResult } from "../animation-account-result";
import { PrecheckAccountResult } from "../precheck-account-result";
import { DetectionResultCard } from "../detection-result-card";

const result: AnimationResult = {
  account_id: "41",
  account_name: "统计账号",
  model: "gpt-6-astra",
  request_id: "metrics",
  completed_at: "2026-09-24T00:00:00Z",
  status: "succeeded",
  duration_ms: 2000,
  generation_duration_ms: 2000,
  svg: '<svg xmlns="http://www.w3.org/2000/svg"/>',
  usage: { input_tokens: 100, output_tokens: 40, total_tokens: 140 },
};

it.each(["card", "fill", "grouped", "precheck", "grouped-precheck"] as const)(
  "%s 结果卡片隐藏用量和 TPS，打开详情后显示完整统计",
  async (layout) => {
    const user = userEvent.setup();
    const precheck = layout.includes("precheck");
    if (layout === "card" || layout === "fill")
      render(
        <AnimationAccountResult
          layout={layout}
          result={result}
          retryDisabled={false}
          onRetry={vi.fn()}
        />,
      );
    else if (layout === "precheck")
      render(<PrecheckAccountResult result={{ ...result, mode: "precheck" }} />);
    else
      render(
        <DetectionResultCard
          row={{
            id: "41",
            name: "统计账号",
            animation: precheck ? undefined : result,
            precheck: precheck ? { ...result, mode: "precheck" } : undefined,
          }}
          active={false}
          precheck={precheck}
          animation={!precheck}
          terminal={false}
        />,
      );
    expect(screen.queryByText("输入 Token")).not.toBeInTheDocument();
    expect(screen.queryByText("输出 Token")).not.toBeInTheDocument();
    expect(screen.queryByText("总计 Token")).not.toBeInTheDocument();
    expect(screen.queryByText("TPS（计算）")).not.toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: precheck ? "查看前置检测详情" : "查看动画检测详情" }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    expect(dialog.getByText("100")).toBeVisible();
    expect(dialog.getByText("40")).toBeVisible();
    expect(dialog.getByText("140")).toBeVisible();
    expect(dialog.getByText("20.0 TPS")).toBeVisible();
    await user.click(dialog.getByRole("button", { name: "关闭" }));
    expect(screen.queryByText("输入 Token")).not.toBeInTheDocument();
  },
);
