import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { AnimationResult } from "@/api";
import { AnimationPreview } from "../animation-preview";

const result = {
  account_id: "41",
  account_name: "动画账号",
  model: "gpt-6-astra",
  request_id: "html-preview",
  status: "succeeded",
  duration_ms: 5000,
  generation_duration_ms: 4000,
  completed_at: "2026-09-24T00:00:00Z",
  html: "<!DOCTYPE html><html><body><svg><circle r='10'/></svg></body></html>",
  source: "<!DOCTYPE html><html><body><svg><circle r='10'/></svg></body></html>",
  prompt: "本次实际发送的提示词",
  reasoning_effort: "low",
  usage: { input_tokens: 1200, output_tokens: 200, total_tokens: 1400 },
} satisfies AnimationResult;

it("HTML 动画可切换代码和本次提示词，不展示用量统计", async () => {
  const user = userEvent.setup();
  render(<AnimationPreview result={result} />);
  await user.click(screen.getByRole("button", { name: /放大查看/ }));
  const dialog = within(await screen.findByRole("dialog", { name: "动画预览" }));
  const frame = dialog.getByLabelText("动画账号生成的鹈鹕骑自行车动画");
  expect(frame).toHaveAttribute("sandbox", "");
  expect(frame).toHaveAttribute("referrerpolicy", "no-referrer");
  expect(frame.getAttribute("srcdoc")).toContain("script-src 'none'");
  expect(dialog.getByText("low")).toBeVisible();
  expect(dialog.queryByText("输入 Token")).not.toBeInTheDocument();
  expect(dialog.queryByText("TPS（计算）")).not.toBeInTheDocument();
  await user.click(dialog.getByRole("tab", { name: "代码" }));
  expect(dialog.getByRole("tabpanel", { name: "代码" })).toHaveTextContent(result.source);
  await user.keyboard("{ArrowRight}{Enter}");
  expect(dialog.getByRole("tab", { name: "提示词" })).toHaveAttribute("aria-selected", "true");
  expect(dialog.getByRole("tabpanel", { name: "提示词" })).toHaveTextContent(result.prompt);
  await user.click(dialog.getByRole("tab", { name: "动画" }));
  expect(dialog.getByLabelText("动画账号生成的鹈鹕骑自行车动画")).toBeVisible();
});

it("历史结果缺失用量和提示词时不补零或套用新提示词", async () => {
  const user = userEvent.setup();
  render(
    <AnimationPreview
      result={{
        ...result,
        usage: undefined,
        prompt: undefined,
        reasoning_effort: undefined,
        generation_duration_ms: undefined,
      }}
    />,
  );
  await user.click(screen.getByRole("button", { name: /放大查看/ }));
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.queryByText("无法计算")).not.toBeInTheDocument();
  expect(dialog.queryByText("low")).not.toBeInTheDocument();
  await user.click(dialog.getByRole("tab", { name: "提示词" }));
  expect(dialog.getByRole("tabpanel")).toHaveTextContent("该历史记录未保存提示词");
});
