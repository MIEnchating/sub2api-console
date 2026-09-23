import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { AnimationResult, Task } from "@/api";
import { account } from "@/features/accounts/__tests__/fixtures";
import { AnimationAccountCard } from "../animation-account-card";

const result: AnimationResult = {
  account_id: "41",
  account_name: "重测账号",
  model: "original-model",
  status: "succeeded",
  svg: '<svg xmlns="http://www.w3.org/2000/svg"/>',
  request_id: "original-request",
  duration_ms: 1000,
  completed_at: "2026-09-22T00:00:00Z",
};

function setup(options: {
  result?: AnimationResult;
  taskStatus?: Task["status"];
  retryDisabled?: boolean;
  platform?: string;
}) {
  const onRetry = vi.fn();
  render(
    <AnimationAccountCard
      account={{ ...account, name: "重测账号", platform: options.platform ?? "openai" }}
      result={options.result}
      taskStatus={options.taskStatus}
      checked={false}
      disabled={false}
      retryDisabled={options.retryDisabled ?? false}
      schedulesReady
      onRetry={onRetry}
      onToggle={vi.fn()}
      onSchedule={vi.fn()}
    />,
  );
  return onRetry;
}

it.each(["succeeded", "failed"] as const)(
  "%s 结果的重测按钮支持键盘操作并沿用原账号与模型",
  async (status) => {
    const user = userEvent.setup();
    const onRetry = setup({ result: { ...result, status, error: "上游超时" } });
    const button = screen.getByRole("button", { name: "重测 重测账号" });
    expect(button).toHaveTextContent("重测");
    button.focus();
    await user.keyboard("{Enter}");
    expect(onRetry).toHaveBeenCalledTimes(1);
    expect(onRetry).toHaveBeenCalledWith({ account_id: "41", model: "original-model" });
  },
);

it.each(["queued", "running", "waiting_input"] as const)(
  "账号任务为 %s 时保留重测入口但禁止重复提交",
  async (taskStatus) => {
    const user = userEvent.setup();
    const onRetry = setup({ result, taskStatus });
    const button = screen.getByRole("button", { name: "重测 重测账号" });
    expect(button).toBeDisabled();
    await user.click(button);
    expect(onRetry).not.toHaveBeenCalled();
  },
);

it("账号处于提交中时重测按钮禁用", () => {
  setup({ result, retryDisabled: true });
  expect(screen.getByRole("button", { name: "重测 重测账号" })).toBeDisabled();
});

it("账号平台已不支持动画检测时禁止重测旧结果", () => {
  setup({ result, platform: "gemini" });
  expect(screen.getByRole("button", { name: "重测 重测账号" })).toBeDisabled();
});

it("未检测的账号显示空状态且不提供无模型的重测入口", () => {
  setup({});
  expect(screen.getByText("尚未检测")).toBeVisible();
  expect(screen.queryByRole("button", { name: "重测 重测账号" })).not.toBeInTheDocument();
});
