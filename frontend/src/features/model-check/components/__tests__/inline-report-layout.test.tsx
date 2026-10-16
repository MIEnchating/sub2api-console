import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { Task } from "@/api";
import { VerificationReport } from "../verification-report";

afterEach(cleanup);

function task(): Task {
  return {
    id: "report",
    skill: "onboarding",
    operation: "onboarding-probe-probe",
    status: "failed",
    progress: 100,
    message: "知识边界请求超时",
    created_at: "",
    updated_at: "",
    result: {},
  };
}
const result = {
  claimed_model: "claude-opus-5",
  total: 2,
  passed: 1,
  capability_total: 1,
  capability: { status: "passed" },
  requests: { successful: 1, total: 2 },
  public_reference: {
    version: "v1",
    status: "inconclusive",
    summary: "1 个请求超时，暂时无法判定",
    knowledge: { status: "inconclusive", valid: 0, total: 10 },
  },
  probes: [
    { id: "math", category: "capability", label: "多步计算", status: "passed", answer: "42" },
    {
      id: "public_kbf_crypto_0",
      category: "public_reference",
      label: "知识边界",
      status: "error",
      error: "请求超时",
    },
  ],
};

it("嵌入报告直接展示分组明细和失败原因，查看来源无需再打开弹窗", async () => {
  render(<VerificationReport task={task()} results={[result]} embedded />);
  expect(screen.queryByRole("button", { name: "查看检测证据" })).not.toBeInTheDocument();
  const group = screen.getByRole("region", { name: "公开参考对比" });
  expect(within(group).getByRole("button", { name: /公开参考对比/ })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  expect(within(group).getByText("请求超时")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "来源与指纹" }));
  expect(screen.getByRole("region", { name: "公开模型指纹对比" })).toBeVisible();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByRole("region", { name: "账号模型检测结果" })).not.toHaveClass("overflow-auto");
});

it("仅看异常可以用键盘切换，清除筛选后保留成功检测项", async () => {
  render(<VerificationReport task={task()} results={[result]} embedded />);
  const filter = screen.getByRole("button", { name: "仅看异常 1" });
  filter.focus();
  await userEvent.keyboard("{Enter}");
  expect(filter).toHaveAttribute("aria-pressed", "true");
  expect(screen.queryByRole("region", { name: "能力检测" })).not.toBeInTheDocument();
  await userEvent.click(filter);
  expect(screen.getByRole("region", { name: "能力检测" })).toBeVisible();
});

it("空报告和超长型号仍保留可读内容且不引入内部滚动面板", () => {
  render(
    <VerificationReport
      task={{ ...task(), status: "succeeded" }}
      results={[{ claimed_model: "long-model-".repeat(30), probes: [] }]}
      embedded
    />,
  );
  expect(screen.getByText("没有逐项记录")).toBeVisible();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(screen.getByRole("region", { name: "账号模型检测结果" })).toHaveClass("min-w-0");
});

it("多个检测组合仍分别展示型号与报告，不把结果合并", () => {
  render(
    <VerificationReport
      task={task()}
      results={[result, { ...result, claimed_model: "other-model" }]}
      embedded
    />,
  );
  expect(screen.getByRole("heading", { name: /claude-opus-5/ })).toBeVisible();
  expect(screen.getByRole("heading", { name: /other-model/ })).toBeVisible();
});

it("展开成功检测项可在当前报告查看完整题目与答案", async () => {
  render(<VerificationReport task={task()} results={[result]} embedded />);
  const group = screen.getByRole("region", { name: "能力检测" });
  await userEvent.click(within(group).getByRole("button", { name: /能力检测/ }));
  await userEvent.click(within(group).getByText("多步计算"));
  expect(await within(group).findByRole("region", { name: "实际回答" })).toHaveTextContent("42");
  expect(within(group).getByRole("region", { name: "检测题目" })).toBeVisible();
});

it("组合没有逐项记录时仍显示该组合的具体错误", () => {
  render(
    <VerificationReport
      task={{ ...task(), status: "partial" }}
      results={[
        {
          error: "当前接口不支持检测参数",
          capability: { status: "inconclusive", summary: "未取得能力回答" },
          probes: [],
        },
      ]}
      embedded
    />,
  );
  expect(screen.getByText("当前接口不支持检测参数")).toBeVisible();
  expect(screen.getByText("未取得能力回答")).toBeVisible();
});
