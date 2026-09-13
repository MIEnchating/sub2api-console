import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { AnimationResult } from "@/api";
import { AnimationAccountResult } from "../animation-account-result";

const result: AnimationResult = {
  account_id: "41",
  account_name: "动画账号",
  model: "test-model",
  request_id: "preview-1",
  status: "succeeded",
  svg: '<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>',
  duration_ms: 1000,
  completed_at: "2026-09-13T00:00:00Z",
};

it("点击卡片动画后打开隔离图片大图，关闭后返回预览入口", async () => {
  const user = userEvent.setup();
  render(<AnimationAccountResult result={result} retryDisabled={false} onRetry={vi.fn()} />);
  const trigger = screen.getByRole("button", { name: "放大查看 动画账号 的动画" });
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  await user.click(within(trigger).getByRole("img"));
  const dialog = await screen.findByRole("dialog", { name: "动画预览" });
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  expect(within(dialog).getByRole("img")).toHaveAttribute(
    "src",
    `data:image/svg+xml;charset=utf-8,${encodeURIComponent(result.svg!)}`,
  );
  expect(dialog.querySelector("iframe")).not.toBeInTheDocument();
  await user.click(within(dialog).getByRole("button", { name: "关闭" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(trigger).toHaveFocus();
  expect(trigger).toHaveAttribute("aria-expanded", "false");
});

it("键盘打开大图后按 Esc 关闭，焦点回到卡片动画", async () => {
  const user = userEvent.setup();
  render(<AnimationAccountResult result={result} retryDisabled={false} onRetry={vi.fn()} />);
  const trigger = screen.getByRole("button", { name: "放大查看 动画账号 的动画" });
  trigger.focus();
  await user.keyboard("{Enter}");
  expect(await screen.findByRole("dialog", { name: "动画预览" })).toBeVisible();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(trigger).toHaveFocus();
});

it("失败或缺少动画内容时不提供放大入口", () => {
  render(
    <AnimationAccountResult
      result={{ ...result, status: "failed", svg: undefined, error: "上游拒绝请求" }}
      retryDisabled={false}
      onRetry={vi.fn()}
    />,
  );
  expect(screen.queryByRole("button", { name: /放大查看/ })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "重试 动画账号" })).toBeEnabled();
});

it("透明动画使用浅色画布，深色主题下黑色线条仍清晰可见", () => {
  render(<AnimationAccountResult result={result} retryDisabled={false} onRetry={vi.fn()} />);
  expect(screen.getByRole("button", { name: "放大查看 动画账号 的动画" })).toHaveClass(
    "bg-slate-100",
  );
});
