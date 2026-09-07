import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { AccountRecovery } from "@/api";
import { AccountRecoveryStatus } from "../account-recovery-status";

const recovery: AccountRecovery = {
  evaluated_at: "2026-09-07T12:00:00Z",
  ready: false,
  conditions: [
    { code: "health_score", met: true, detail: "健康分 100/75 分" },
    { code: "success_streak", met: false, detail: "连续成功 1/2 次" },
    { code: "fuse_cooldown", met: false, detail: "熔断冷却剩余 30 秒" },
  ],
};

describe("account recovery progress", () => {
  afterEach(cleanup);

  it("shows unmet conditions in the compact summary even at full score", () => {
    render(<AccountRecoveryStatus recovery={recovery} />);
    expect(screen.getByText("恢复待满足：连续成功 1/2 次；熔断冷却剩余 30 秒")).toBeVisible();
  });

  it("shows each gate and the evaluation time in expanded details", () => {
    render(<AccountRecoveryStatus recovery={recovery} expanded />);
    const list = screen.getByRole("list");
    const conditions = within(list).getAllByRole("listitem");
    expect(within(conditions[0]).getByText("已满足")).toBeVisible();
    expect(within(conditions[1]).getByText("未满足")).toBeVisible();
    expect(screen.getByText(/评估时间：2026-09-07T12:00:00Z/)).toBeVisible();
  });

  it("waits for execution instead of claiming the account recovered when all gates pass", () => {
    render(<AccountRecoveryStatus recovery={{ ...recovery, ready: true, conditions: [] }} />);
    expect(screen.getByText("恢复条件已满足，等待调度执行")).toBeVisible();
    expect(screen.queryByText("已恢复")).not.toBeInTheDocument();
  });
});
