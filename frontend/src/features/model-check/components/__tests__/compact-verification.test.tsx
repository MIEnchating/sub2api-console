import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { VerificationCombinationProgress } from "../verification-combination-progress";

const result = {
  account_name: "接入前探活",
  claimed_model: "selected-model",
  planned_probes: 60,
  timeout_seconds: 45,
  probes: [
    {
      id: "done",
      label: "已采样题目",
      category: "public_reference",
      status: "observed",
      answer: "1926",
    },
    {
      id: "error",
      label: "失败题目",
      category: "public_reference",
      status: "error",
      error: "请求超时",
    },
  ],
  current_probe: {
    id: "current",
    label: "当前参考题",
    category: "public_reference",
    status: "running",
  },
  probe_plan: [
    { id: "done", label: "已采样题目", category: "public_reference" },
    { id: "error", label: "失败题目", category: "public_reference" },
    { id: "current", label: "当前参考题", category: "public_reference" },
    { id: "next", label: "下一道参考题", category: "public_reference" },
  ],
};

it("紧凑检测默认显示当前项和统计，分组明细按需展开且不重复账号信息", async () => {
  render(<VerificationCombinationProgress result={result} active compact />);
  expect(screen.getByText("已执行 2/60 项")).toBeVisible();
  expect(screen.queryByRole("heading", { name: "接入前探活" })).not.toBeInTheDocument();
  const current = screen.getByRole("region", { name: "当前检测" });
  expect(within(current).getByText("当前参考题", { selector: "span[title]" })).toBeVisible();
  const group = screen.getByRole("region", { name: "公开参考对比" });
  expect(
    within(group).queryByText("下一道参考题", { selector: "span[title]" }),
  ).not.toBeInTheDocument();
  const toggle = within(group).getByRole("button", { name: /公开参考对比/ });
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  toggle.focus();
  await userEvent.keyboard("{Enter}");
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  expect(within(group).getByText("下一道参考题", { selector: "span[title]" })).toBeVisible();
  expect(within(group).getByText("请求超时")).toBeVisible();
  expect(within(group).queryByText("1926")).not.toBeVisible();
  await userEvent.click(within(group).getByText("已采样题目", { selector: "span[title]" }));
  expect(within(group).getByText("1926")).toBeVisible();
});

it("检测更新时保持手动展开状态并更新当前项，结束后不显示过期的当前请求", async () => {
  const view = render(<VerificationCombinationProgress result={result} active compact />);
  await userEvent.click(screen.getByRole("button", { name: /公开参考对比/ }));
  view.rerender(
    <VerificationCombinationProgress
      result={{ ...result, current_probe: { ...result.current_probe, label: "后续参考题" } }}
      active
      compact
    />,
  );
  expect(screen.getByRole("button", { name: /公开参考对比/ })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  expect(
    within(screen.getByRole("region", { name: "当前检测" })).getByText("后续参考题", {
      selector: "span[title]",
    }),
  ).toBeVisible();
  view.rerender(<VerificationCombinationProgress result={result} active={false} compact />);
  expect(screen.queryByRole("region", { name: "当前检测" })).not.toBeInTheDocument();
});

it("空检测计划显示等待提示且不伪造分组或进度条", () => {
  render(<VerificationCombinationProgress result={{}} active compact />);
  expect(screen.getByText(/等待单项检测进度/)).toBeVisible();
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
});

it("多个请求并发时当前检测同时显示全部项目，完成项从运行区移出", () => {
  const first = { id: "first", label: "计算检测", status: "running" };
  const second = { id: "second", label: "逻辑检测", status: "running" };
  const view = render(
    <VerificationCombinationProgress result={{ current_probes: [first, second] }} active compact />,
  );
  const current = screen.getByRole("region", { name: "当前检测" });
  expect(within(current).getByText("计算检测", { selector: "span[title]" })).toBeVisible();
  expect(within(current).getByText("逻辑检测", { selector: "span[title]" })).toBeVisible();
  expect(within(current).getByText(/2 项进行中/)).toBeVisible();
  view.rerender(
    <VerificationCombinationProgress
      result={{ current_probes: [second], probes: [{ ...first, status: "passed" }] }}
      active
      compact
    />,
  );
  expect(within(current).queryByText("计算检测")).not.toBeInTheDocument();
  expect(within(current).getByText(/1 项进行中/)).toBeVisible();
});
