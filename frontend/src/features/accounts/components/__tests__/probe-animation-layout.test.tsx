import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { ProbeAnimationResult } from "../probe-animation-result";

it("动画完成后仅展示一条结果摘要和秒数，过程默认收起且可用键盘展开", async () => {
  render(
    <ProbeAnimationResult
      pending={false}
      result={{
        status: "passed",
        message: "动画测试完成",
        request_model: "selected-model",
        actual_model: "actual-model",
        latency_ms: 58492,
        http_status: 0,
      }}
      steps={[{ stage: "cleanup_key", status: "succeeded", started_at: "2026-10-13T00:00:00Z" }]}
    />,
  );
  expect(screen.getByText("动画已生成")).toBeVisible();
  expect(screen.getByText("58.5 秒")).toBeVisible();
  expect(screen.queryByText(/HTTP/)).not.toBeInTheDocument();
  expect(screen.queryByText("动画测试完成")).not.toBeInTheDocument();
  expect(screen.queryByRole("list", { name: "探活过程" })).not.toBeInTheDocument();
  const toggle = screen.getByRole("button", { name: "查看测试过程" });
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  toggle.focus();
  await userEvent.keyboard("{Enter}");
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByRole("list", { name: "探活过程" })).toBeVisible();
  expect(screen.getByText(/actual-model/)).toBeVisible();
});

it("动画失败时保留具体原因且画布维持四比三比例", () => {
  render(
    <ProbeAnimationResult
      pending={false}
      result={{
        status: "failed",
        message: "上游额度不足，请充值后重试",
        request_model: "model",
        actual_model: "",
        latency_ms: 0,
        http_status: 403,
      }}
      steps={[]}
    />,
  );
  expect(screen.getByText("上游额度不足，请充值后重试")).toBeVisible();
  expect(screen.getByRole("region", { name: "动画展示区域" })).toHaveClass(
    "aspect-[4/3]",
    "mx-auto",
  );
});
