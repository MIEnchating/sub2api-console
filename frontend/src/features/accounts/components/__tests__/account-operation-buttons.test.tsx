import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AccountStatus } from "@/api";
import { AccountOperationButtons } from "../account-operation-buttons";
import { AccountOperationControls } from "../account-operation-controls";

const account: AccountStatus = {
  id: "41",
  name: "channel-41",
  groups: ["codex"],
  upstream_id: "up_test",
  upstream_host: "api.example.test",
  upstream_type: "apikey",
  schedulable: true,
  priority: 1,
  load_factor: "10",
  concurrency: 2,
  multiplier: "0.1",
  balance: null,
  paused: false,
  paused_reason: null,
  routing_state: "healthy",
  health_status: "healthy",
  health: "healthy",
  desired_health: "healthy",
  apply_pending: false,
  apply_error: null,
  decision_state: "healthy",
  decision_reason: null,
  failure_streak: 0,
  recovery_pass_streak: 0,
  target_priority: 1,
  target_load_factor: "10",
  target_schedulable: true,
  target_concurrency: 2,
  health_score: 100,
  short_score: 100,
  long_score: 100,
  sample_count: 1,
  recent_results: [],
  ttfb_p50_ms: 100,
  ttfb_p95_ms: 120,
  weight: 100,
};

function markup(overrides: Partial<AccountStatus> = {}, probePending = false) {
  return renderToStaticMarkup(
    <AccountOperationControls
      expanded
      account={{ ...account, ...overrides }}
      pending={probePending}
      probePending={probePending}
      onProbe={vi.fn()}
      onControl={vi.fn()}
      onRateSync={vi.fn()}
      onManualPriority={vi.fn()}
      onEdit={vi.fn()}
      onDelete={vi.fn()}
    />,
  );
}

describe("account operation buttons", () => {
  afterEach(cleanup);

  it("opens complete failure details and common actions from one account entry", async () => {
    const onProbe = vi.fn();
    render(
      <AccountOperationButtons
        account={{
          ...account,
          sub2api_status: "error",
          sub2api_error: "上游返回 503，请稍后重新探活",
        }}
        pending={false}
        probePending={false}
        onProbe={onProbe}
        onControl={vi.fn()}
        onRateSync={vi.fn()}
        onManualPriority={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "状态与处置" }));
    const dialog = await screen.findByRole("dialog", { name: "状态与处置" });
    expect(within(dialog).getByText("最近错误：上游返回 503，请稍后重新探活")).toBeVisible();
    const details = within(dialog).getByRole("region", { name: "账号状态详情" });
    expect(details).toHaveClass("min-h-0", "overflow-y-auto");
    expect(within(details).queryByRole("group", { name: "账号常用处置" })).not.toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "探活测试" }));
    expect(onProbe).toHaveBeenCalledOnce();
  });

  it("keeps disposition details readable while probe and control actions are pending", async () => {
    render(
      <AccountOperationButtons
        account={account}
        pending
        probePending
        onProbe={vi.fn()}
        onControl={vi.fn()}
        onRateSync={vi.fn()}
        onManualPriority={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "状态与处置" }));
    const dialog = await screen.findByRole("dialog", { name: "状态与处置" });
    expect(within(dialog).getByRole("button", { name: "正在探活" })).toBeDisabled();
    expect(within(dialog).getByRole("button", { name: "手动熔断（停止调度）" })).toBeDisabled();
    expect(within(dialog).queryByRole("button", { name: /取消/ })).not.toBeInTheDocument();
  });

  it("passes fuse confirmation from the disposition panel to the existing control flow", async () => {
    const onControl = vi.fn();
    render(
      <AccountOperationButtons
        account={account}
        pending={false}
        probePending={false}
        onProbe={vi.fn()}
        onControl={onControl}
        onRateSync={vi.fn()}
        onManualPriority={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "状态与处置" }));
    const dialog = await screen.findByRole("dialog", { name: "状态与处置" });
    fireEvent.click(within(dialog).getByRole("button", { name: "手动熔断（停止调度）" }));
    expect(onControl).toHaveBeenCalledWith(
      "fuse",
      "手动熔断",
      expect.stringContaining("直到手动解除"),
    );
  });

  it("opens by keyboard with focus on the explanation title and restores focus on Escape", async () => {
    const user = userEvent.setup();
    render(
      <AccountOperationButtons
        account={account}
        pending={false}
        probePending={false}
        onProbe={vi.fn()}
        onControl={vi.fn()}
        onRateSync={vi.fn()}
        onManualPriority={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
      />,
    );
    const trigger = screen.getByRole("button", { name: "状态与处置" });
    trigger.focus();
    await user.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog", { name: "状态与处置" });
    await waitFor(() =>
      expect(within(dialog).getByRole("heading", { name: "状态与处置" })).toHaveFocus(),
    );
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(trigger).toHaveAttribute("aria-expanded", "false");
  });

  it("offers recovery with confirmation for a fused account in the disposition panel", async () => {
    const onControl = vi.fn();
    render(
      <AccountOperationButtons
        account={{ ...account, health: "fused", schedulable: false }}
        pending={false}
        probePending={false}
        onProbe={vi.fn()}
        onControl={onControl}
        onRateSync={vi.fn()}
        onManualPriority={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "状态与处置" }));
    const dialog = await screen.findByRole("dialog", { name: "状态与处置" });
    fireEvent.click(within(dialog).getByRole("button", { name: "解除熔断" }));
    expect(onControl).toHaveBeenCalledWith(
      "recover",
      "解除熔断",
      expect.stringContaining("仍受调度策略约束"),
    );
  });
  it("matches the channel pool operations without a multiplier threshold breaker", () => {
    const result = markup();
    for (const label of ["探活测试", "暂停调度", "手动熔断（停止调度）"]) {
      expect(result).toContain(label);
    }
    expect(result).toContain('aria-label="更多账号操作"');
    expect(result).toContain("lucide-ellipsis");
    expect(result).toContain("flex");
    expect(result).not.toContain("grid-cols-4");
    expect(result).not.toContain("w-[9.5rem]");
    expect(result).toContain("lucide-activity");
    expect(result).not.toContain("lucide-scan-search");
    expect(result).not.toContain("排除账号");
    expect(result).not.toContain("倍率超阈值");
    expect(result.match(/border-destructive\/40/g)).toHaveLength(1);
    expect(result).toContain("bg-destructive/10");
    expect(result).toContain("hover:bg-destructive/20");
  });

  it("shows an adjustment action for an assigned manual priority account", () => {
    const result = markup({ manual_priority: 3, manual_sync_balance_multiplier: false });
    expect(result).toContain('aria-label="更多账号操作"');
    expect(result.match(/disabled/g)?.length).toBeGreaterThanOrEqual(4);
    expect(result).not.toContain('aria-label="同步账号倍率"');
  });

  it("always enables cost sync for a manual account", () => {
    const withoutBalanceSync = markup({
      manual_priority: 3,
      manual_sync_balance_multiplier: false,
    });
    const withBalanceSync = markup({
      manual_priority: 3,
      manual_sync_balance_multiplier: true,
    });

    expect(withoutBalanceSync).not.toMatch(/disabled=""[^>]*aria-label="同步账号倍率"/);
    expect(withBalanceSync).not.toMatch(/disabled=""[^>]*aria-label="同步账号倍率"/);
  });

  it("shows an immediate loading state while an active probe is running", () => {
    const result = markup({}, true);

    expect(result).toContain('aria-label="正在探活"');
    expect(result).toContain("lucide-loader-circle");
    expect(result).toContain("animate-spin");
    expect(result).toContain("disabled");
    expect(result).not.toContain("lucide-activity");
  });

  it("switches pause and fuse actions to their recovery variants", () => {
    expect(markup({ health: "paused", routing_state: "paused", paused: true })).toContain(
      "恢复调度",
    );
    const fused = markup({ health: "fused", routing_state: "fused", schedulable: false });
    expect(fused).toContain("解除熔断");
    expect(fused.match(/border-destructive\/40/g) ?? []).toHaveLength(0);
  });

  it("does not offer pause again when a policy already stopped scheduling", () => {
    const result = markup({
      health: "cost_blocked",
      routing_state: "cost_blocked",
      decision_state: "cost_blocked",
      schedulable: false,
      target_schedulable: false,
    });

    expect(result).not.toContain('aria-label="已停止调度"');
    expect(result).not.toContain('aria-label="暂停调度"');
  });

  it("offers recovery when Sub2API reports an otherwise disabled account", () => {
    const result = markup({
      health: "disabled",
      routing_state: "disabled",
      schedulable: false,
    });

    expect(result).toContain('aria-label="恢复调度"');
    expect(result).not.toContain('aria-label="暂停调度"');
  });

  it("keeps action labels independent from the account name", () => {
    expect(markup()).not.toContain(account.name);
  });

  it("keeps read-only rate sync available for an excluded account", () => {
    const result = markup({ health: "excluded", routing_state: "excluded", schedulable: false });
    expect(result).toContain("恢复管控");
    expect(result).toContain('aria-label="更多账号操作"');
    expect(result).not.toContain("探活测试");
    expect(result).not.toContain("手动熔断");
  });
});
