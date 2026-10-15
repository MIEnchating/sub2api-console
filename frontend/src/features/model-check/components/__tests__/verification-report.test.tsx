import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { Task } from "@/api";
import { ModelCheckResult } from "../model-check-result";

const result = {
  account_id: "41",
  account_name: "测试账号",
  claimed_model: "new-model",
  checker: "verification",
  verdict: "NO_ANOMALIES",
  capability: { status: "passed", summary: "本次题目全部通过" },
  authenticity: { status: "unverified", summary: "缺少可信身份依据" },
  passed: 2,
  total: 2,
  requests: { successful: 2, total: 2 },
  probes: [
    {
      id: "arithmetic",
      label: "多步计算",
      round: 1,
      status: "passed",
      expected: "21",
      answer: "**21**",
      prompt: "7 × 3",
      request_id: "req-1",
    },
    {
      id: "constraints",
      label: "约束推理",
      round: 1,
      status: "passed",
      expected: "42",
      answer: "42",
      prompt: "约束题目",
      request_id: "req-2",
    },
  ],
};
const task: Task = {
  id: "report",
  skill: "model-check",
  operation: "check",
  status: "succeeded",
  progress: 100,
  message: "完成",
  created_at: "2026-10-15T00:00:00Z",
  updated_at: "2026-10-15T00:01:00Z",
  result: { tests: [result] },
};

it("全部检测通过时使用成功状态且把未认证身份显示为中性说明", () => {
  render(<ModelCheckResult task={task} />);
  const results = screen.getByRole("region", { name: "账号模型检测结果" });
  expect(within(results).getByText("能力通过").closest('[data-slot="badge"]')).toHaveAttribute(
    "data-variant",
    "success",
  );
  expect(screen.getByText("接口信息未记录").closest('[data-slot="badge"]')).toHaveAttribute(
    "data-variant",
    "neutral",
  );
  expect(screen.getByText(/能力、请求来源与指纹稳定性分别展示/)).toBeVisible();
  expect(screen.queryByText("身份异常")).not.toBeInTheDocument();
  expect(screen.queryByRole("columnheader", { name: "结论" })).not.toBeInTheDocument();
});

it("证据弹窗默认展示题目与原文对照并可切换检测项", async () => {
  const user = userEvent.setup();
  render(<ModelCheckResult task={task} />);
  await user.click(screen.getByRole("button", { name: "查看检测证据" }));
  const dialog = screen.getByRole("dialog", { name: "检测证据" });
  expect(within(dialog).getByText("**21**", { exact: true })).toBeVisible();
  expect(within(dialog).getByText("7 × 3", { exact: true })).toBeVisible();
  await user.click(within(dialog).getByRole("button", { name: /第 1 轮.*约束推理/ }));
  expect(within(dialog).getByText("约束题目", { exact: true })).toBeVisible();
  expect(within(dialog).queryByText("**21**", { exact: true })).not.toBeInTheDocument();
  expect(within(dialog).getByRole("button", { name: /第 1 轮.*约束推理/ })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
});

it("请求失败和答题未通过保持独立结论", () => {
  render(
    <ModelCheckResult
      task={{
        ...task,
        result: {
          tests: [
            {
              ...result,
              capability: { status: "inconclusive" },
              verdict: "ERROR",
              requests: { successful: 0, total: 1 },
              error: "请求限流，请稍后重试",
            },
          ],
        },
      }}
    />,
  );
  expect(screen.getByText("请求失败")).toBeVisible();
  expect(
    within(screen.getByRole("region", { name: "账号模型检测结果" })).getByText("能力待判定"),
  ).toBeVisible();
  expect(screen.getByText("请求限流，请稍后重试")).toBeVisible();
  expect(
    within(screen.getByRole("region", { name: "账号模型检测结果" })).queryByText("能力未通过"),
  ).not.toBeInTheDocument();
});

it("能力通过但返回型号存在差异时仍展示待核对证据", () => {
  render(
    <ModelCheckResult
      task={{
        ...task,
        result: {
          tests: [
            {
              ...result,
              evidence: [
                { id: "model", status: "warning", label: "返回型号", detail: "请求与返回型号不同" },
              ],
            },
          ],
        },
      }}
    />,
  );
  expect(screen.getByText("有待核对证据").closest('[data-slot="badge"]')).toHaveAttribute(
    "data-variant",
    "warning",
  );
  expect(
    within(screen.getByRole("region", { name: "账号模型检测结果" })).getByText("能力通过"),
  ).toBeVisible();
});

it("多组合结果可翻页查看且少量结果不显示分页控件", async () => {
  const user = userEvent.setup();
  render(
    <ModelCheckResult
      task={{
        ...task,
        result: {
          tests: Array.from({ length: 21 }, (_, index) => ({
            ...result,
            account_id: String(index + 1),
            account_name: `账号 ${index + 1}`,
          })),
        },
      }}
    />,
  );
  expect(screen.getByText("账号 20", { exact: true })).toBeVisible();
  expect(screen.queryByText("账号 21", { exact: true })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "转到第 2 页" }));
  expect(screen.getByText("账号 21", { exact: true })).toBeVisible();
});
