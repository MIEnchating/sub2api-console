import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { OnboardingConfirmDialog } from "../onboarding-confirm-dialog";

afterEach(cleanup);

it("仅更新已有绑定时显示目标分组并保持原并发和优先级", () => {
  render(
    <OnboardingConfirmDialog
      open
      items={[
        {
          id: "existing-41",
          host: "models.test",
          upstreamGroupId: "7",
          upstreamGroup: "codex-special",
          platform: "OpenAI",
          multiplier: "0.15",
          localGroup: "codex",
          concurrency: 100,
          priority: 10,
          status: "待更新",
        },
      ]}
      pending={false}
      onOpenChange={() => undefined}
      onConfirm={() => undefined}
    />,
  );
  const account = screen.getByRole("region", { name: "codex-special → codex" });
  expect(within(account).getByText("OpenAI")).toBeVisible();
  expect(within(account).getByText("0.15")).toBeVisible();
  expect(within(account).getAllByText("保持原值")).toHaveLength(2);
  expect(within(account).getByText("待更新")).toBeVisible();
  expect(within(account).queryByRole("textbox")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认提交 1 项变更" })).toBeEnabled();
});

it("没有待确认账号时不允许提交", () => {
  render(
    <OnboardingConfirmDialog
      open
      items={[]}
      pending={false}
      onOpenChange={() => undefined}
      onConfirm={() => undefined}
    />,
  );
  expect(screen.getByRole("button", { name: "确认提交 0 项变更" })).toBeDisabled();
});
