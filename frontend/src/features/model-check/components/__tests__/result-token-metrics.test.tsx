import { render, screen, within } from "@testing-library/react";
import { expect, it } from "vitest";
import userEvent from "@testing-library/user-event";
import type { AnimationResult } from "@/api";
import { ResultTokenMetrics } from "../result-token-metrics";

const result: AnimationResult = {
  account_id: "41",
  account_name: "统计账号",
  model: "gpt-6-astra",
  request_id: "metrics",
  completed_at: "2026-09-24T00:00:00Z",
  status: "succeeded",
  duration_ms: 20000,
  generation_duration_ms: 2000,
  usage: { input_tokens: 100, output_tokens: 40 },
};

it("含失败重试的结果只使用本次生成耗时计算 TPS，完整输入输出可计算总计", () => {
  render(<ResultTokenMetrics result={result} />);
  expect(screen.getByText("20.0 TPS")).toBeVisible();
  expect(screen.getByText("140")).toBeVisible();
  expect(screen.getByText("TPS（计算）")).toBeVisible();
  expect(screen.getByText("输入 Token").closest("dl")).toHaveClass("grid-cols-4", "w-full");
});

it.each([undefined, 0])("耗时为 %s 时不使用总耗时替代，并说明无法计算原因", (duration) => {
  render(<ResultTokenMetrics result={{ ...result, generation_duration_ms: duration }} />);
  expect(screen.getByLabelText("缺少有效生成耗时，无法计算 TPS")).toHaveTextContent("无法计算");
});

it("缺少输出用量时不补零或推算总计，并说明无法计算原因", () => {
  render(<ResultTokenMetrics result={{ ...result, usage: { input_tokens: 100 } }} />);
  expect(screen.getByLabelText("缺少输出 Token，无法计算 TPS")).toHaveTextContent("无法计算");
  const total = screen.getByText("总计 Token").parentElement!;
  expect(within(total).getByText("未返回")).toBeVisible();
});

it("明确返回零输出且耗时有效时显示零 TPS", () => {
  render(<ResultTokenMetrics result={{ ...result, usage: { output_tokens: 0 } }} />);
  expect(screen.getByText("0.0 TPS")).toBeVisible();
});

it("等待检测结果时保留四列占位，不显示旧数据或虚构零用量", () => {
  render(<ResultTokenMetrics />);
  expect(screen.getAllByText("—")).toHaveLength(4);
  expect(screen.queryByText("未返回")).not.toBeInTheDocument();
});

it("悬停 TPS 时显示无法计算原因的共享提示", async () => {
  const user = userEvent.setup();
  render(<ResultTokenMetrics result={{ ...result, usage: undefined }} />);
  await user.hover(screen.getByLabelText("缺少输出 Token，无法计算 TPS"));
  expect(await screen.findByText("缺少输出 Token，无法计算 TPS")).toBeVisible();
});
