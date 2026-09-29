import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { AnimationResult, TerminalContinuityResult } from "@/api";
import { DetectionResultCard } from "../detection-result-card";

afterEach(cleanup);

const result: AnimationResult = {
  account_id: "41",
  account_name: "检测账号",
  model: "test-model",
  request_id: "request-1",
  status: "succeeded",
  duration_ms: 10,
  completed_at: "2026-09-24T00:00:00Z",
};

it.each([
  ["passed", "通过", "text-success", "bg-success/10"],
  ["not_passed", "降智", "text-destructive", "bg-destructive/10"],
  ["inconclusive", "无法判定", "text-warning", "bg-warning/10"],
  ["error", "请求失败", "text-destructive", "bg-destructive/10"],
] as const)("前置结果为 %s 时用对应颜色和文字展示 %s", (verdict, label, color, background) => {
  render(
    <DetectionResultCard
      row={{
        id: "41",
        name: "检测账号",
        precheck: {
          ...result,
          mode: "precheck",
          precheck: { verdict, profile_version: "candy-v1", questions: [] },
        },
      }}
      active={false}
      animation={false}
      precheck
      terminal={false}
    />,
  );
  const status = screen.getByRole("group", { name: `前置检测 · ${label}` });
  expect(status).toHaveClass(color, background);
  expect(status).toHaveTextContent(`前置检测 · ${label}`);
  expect(status.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  expect(screen.getByRole("button", { name: "查看前置检测详情" })).toBeEnabled();
});

it.each([
  ["normal", "正常", "text-success"],
  ["suspected", "疑似无终端权限", "text-destructive"],
  ["inconclusive", "证据不足", "text-warning"],
  ["error", "请求失败", "text-destructive"],
] as const)("终端结果为 %s 时用对应颜色和文字展示 %s", (verdict, label, color) => {
  const terminal: TerminalContinuityResult = { ...result, verdict };
  render(
    <DetectionResultCard
      row={{ id: "41", name: "检测账号", terminal }}
      active={false}
      animation={false}
      precheck={false}
      terminal
    />,
  );
  expect(screen.getByRole("group", { name: `终端检测 · ${label}` })).toHaveClass(color);
});

it.each([
  ["succeeded", "成功", "text-success"],
  ["failed", "失败", "text-destructive"],
] as const)("动画结果为 %s 时用对应颜色和文字展示 %s", (status, label, color) => {
  render(
    <DetectionResultCard
      row={{ id: "41", name: "检测账号", animation: { ...result, status, svg: "<svg/>" } }}
      active={false}
      animation
      precheck={false}
      terminal={false}
    />,
  );
  expect(screen.getByRole("group", { name: `动画检测 · ${label}` })).toHaveClass(color);
});

it.each([
  [true, "等待检测结果", "text-muted-foreground"],
  [false, "结果未记录", "text-warning"],
] as const)("尚无结果且运行状态为 %s 时标明 %s 并使用对应颜色", (active, label, color) => {
  render(
    <DetectionResultCard
      row={{ id: "41", name: "检测账号" }}
      active={active}
      animation={false}
      precheck
      terminal={false}
    />,
  );
  expect(screen.getByRole("group", { name: `前置检测 · ${label}` })).toHaveClass(color);
});

it("前置请求失败且未返回报告时使用红色请求失败标记", () => {
  render(
    <DetectionResultCard
      row={{
        id: "41",
        name: "检测账号",
        precheck: { ...result, status: "failed", error: "请求超时" },
      }}
      active={false}
      animation={false}
      precheck
      terminal={false}
    />,
  );
  expect(screen.getByRole("group", { name: "前置检测 · 请求失败" })).toHaveClass(
    "text-destructive",
  );
});
