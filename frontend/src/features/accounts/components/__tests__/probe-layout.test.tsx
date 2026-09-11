import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { ProbeProgressSummary } from "../probe-task-timeline";
import { ProbeResultSlot } from "../account-probe-dialog";

afterEach(cleanup);

it("进度默认仅展示当前阶段，键盘展开后可查看全部真实步骤", async () => {
  const user = userEvent.setup();
  render(
    <ProbeProgressSummary
      steps={[
        { stage: "create_key", status: "succeeded", started_at: "2026-09-11T00:00:00Z" },
        { stage: "models", status: "running", started_at: "2026-09-11T00:00:01Z" },
      ]}
    />,
  );
  const toggle = screen.getByRole("button", { name: /获取上游模型列表/ });
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByRole("list", { name: "探活过程" })).not.toBeInTheDocument();
  toggle.focus();
  await user.keyboard("{Enter}");
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByRole("list", { name: "探活过程" })).toBeInTheDocument();
  expect(screen.getByText("创建临时上游 Key")).toBeVisible();
});

it("测试尚未开始时结果区域默认展示当前模型和等待提示", () => {
  render(<ProbeResultSlot pending={false} error={null} result={null} requestModel="kimi-k2" />);
  expect(screen.getByText(/kimi-k2/)).toBeVisible();
  expect(screen.getByText("点击“开始测试”查看响应")).toBeVisible();
});

it("清理失败时折叠摘要直接显示失败阶段", () => {
  render(
    <ProbeProgressSummary
      steps={[{ stage: "cleanup_key", status: "failed", started_at: "2026-09-11T00:00:00Z" }]}
    />,
  );
  expect(screen.getByRole("button", { name: /清理临时上游 Key.*失败/ })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
});
