import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { account } from "@/features/accounts/__tests__/fixtures";
import { AnimationAccountCard } from "../animation-account-card";

it.each(["123456789", "987654321"])("调度参数 %s 截断后可通过键盘读取完整值", async (value) => {
  const user = userEvent.setup();
  render(
    <AnimationAccountCard
      account={{ ...account, priority: 123456789, weight: 987654321 }}
      checked={false}
      disabled={false}
      retryDisabled={false}
      schedulesReady
      onRetry={vi.fn()}
      onToggle={vi.fn()}
      onSchedule={vi.fn()}
    />,
  );
  const metric = within(screen.getByRole("group", { name: "账号调度参数" })).getByText(value);
  expect(metric).toHaveAttribute("tabindex", "0");
  await user.tab();
  await user.tab();
  if (value === "987654321") await user.tab();
  expect(metric).toHaveFocus();
  await waitFor(() => expect(screen.getAllByText(value)).toHaveLength(2));
  expect(screen.getAllByText(value)[1]).toBeVisible();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.getAllByText(value)).toHaveLength(1));
  expect(metric).toHaveFocus();
});
