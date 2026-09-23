import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { AnimationAccountCard } from "../animation-account-card";
import { account } from "@/features/accounts/__tests__/fixtures";

it("动画与前置模式分别只展示对应结果", () => {
  const result = {
    account_id: "41",
    account_name: "测试账号",
    model: "gpt-6-astra",
    status: "succeeded" as const,
    svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400" />',
    request_id: "animation",
    duration_ms: 10,
    completed_at: "2026-09-15T00:00:00Z",
  };
  const view = render(
    <AnimationAccountCard
      account={{ ...account, id: "41", name: "测试账号" }}
      result={result}
      precheckResult={{
        ...result,
        mode: "precheck",
        precheck: {
          verdict: "passed",
          profile_version: "astra-v1",
          questions: [
            { id: "candy", answer: "21", verdict: "passed", request_id: "candy" },
            {
              id: "knowledge-cutoff",
              answer: "无法提供日期",
              verdict: "passed",
              request_id: "knowledge",
            },
          ],
        },
      }}
      checked={false}
      disabled={false}
      retryDisabled={false}
      schedulesReady
      onRetry={vi.fn()}
      onToggle={vi.fn()}
      onSchedule={vi.fn()}
    />,
  );
  expect(screen.getByRole("article")).toHaveClass("h-auto", "overflow-hidden");
  const preview = screen.getByRole("group", { name: "动画预览区域" });
  expect(preview).toHaveClass("h-[180px]", "shrink-0");
  expect(screen.queryByRole("region", { name: "前置检测结果" })).not.toBeInTheDocument();

  view.rerender(
    <AnimationAccountCard
      mode="precheck"
      account={{ ...account, id: "41", name: "测试账号" }}
      result={result}
      precheckResult={{
        ...result,
        mode: "precheck",
        precheck: {
          verdict: "passed",
          profile_version: "astra-v1",
          questions: [
            { id: "candy", answer: "21", verdict: "passed", request_id: "candy" },
            {
              id: "knowledge-cutoff",
              answer: "无法提供日期",
              verdict: "passed",
              request_id: "knowledge",
            },
          ],
        },
      }}
      checked={false}
      disabled={false}
      retryDisabled={false}
      schedulesReady
      onRetry={vi.fn()}
      onToggle={vi.fn()}
      onSchedule={vi.fn()}
    />,
  );
  const precheck = screen.getByRole("region", { name: "前置检测结果" });
  expect(precheck.parentElement).toHaveClass("min-h-14");
  expect(screen.queryByRole("group", { name: "动画预览区域" })).not.toBeInTheDocument();
  expect(screen.queryByText("无法提供日期")).not.toBeInTheDocument();
});

it("失败卡片限制长错误摘要，键盘可查看完整错误和请求信息且关闭后恢复焦点", async () => {
  const user = userEvent.setup();
  const error = "上游拒绝了此请求，请检查绑定 Key 的权限和请求额度。".repeat(20);
  render(
    <AnimationAccountCard
      account={{ ...account, id: "41", name: "失败账号" }}
      result={{
        account_id: "41",
        account_name: "失败账号",
        model: "gpt-6-astra",
        status: "failed",
        error,
        request_id: "failed-request-41",
        duration_ms: 120000,
        completed_at: "2026-09-15T00:00:00Z",
      }}
      checked={false}
      disabled={false}
      retryDisabled={false}
      schedulesReady
      onRetry={vi.fn()}
      onToggle={vi.fn()}
      onSchedule={vi.fn()}
    />,
  );
  const card = screen.getByRole("article");
  expect(within(card).getByText(error)).toHaveClass("line-clamp-2", "wrap-anywhere");
  expect(within(card).queryByText("failed-request-41")).not.toBeInTheDocument();
  const trigger = within(card).getByRole("button", { name: "查看动画检测详情" });
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  trigger.focus();
  await user.keyboard("{Enter}");
  const dialog = await screen.findByRole("dialog", { name: "动画检测详情" });
  expect(within(dialog).getAllByText(error)).toHaveLength(1);
  expect(within(dialog).getByText("failed-request-41")).toBeVisible();
  expect(within(dialog).getByText(error)).toHaveClass("whitespace-pre-wrap", "wrap-anywhere");
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(trigger).toHaveFocus();
  expect(trigger).toHaveAttribute("aria-expanded", "false");
});

it("账号卡片提供手动控制入口并回传当前账号", async () => {
  const user = userEvent.setup();
  const onManualPriority = vi.fn();
  render(
    <AnimationAccountCard
      account={{ ...account, id: "41", name: "优先账号" }}
      checked={false}
      disabled={false}
      retryDisabled={false}
      schedulesReady
      onRetry={vi.fn()}
      onToggle={vi.fn()}
      onSchedule={vi.fn()}
      onManualPriority={onManualPriority}
    />,
  );

  await user.click(screen.getByRole("button", { name: "设置手动控制" }));
  expect(onManualPriority).toHaveBeenCalledWith(expect.objectContaining({ id: "41" }));
});
