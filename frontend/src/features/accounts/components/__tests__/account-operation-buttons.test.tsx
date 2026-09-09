import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AccountStatus } from "@/api";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { AccountOperationProps } from "../account-operation-controls";
import { AccountOperationButtons } from "../account-operation-buttons";

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

function operationProps(overrides: Partial<AccountStatus> = {}): AccountOperationProps {
  return {
    account: { ...account, ...overrides },
    pending: false,
    probePending: false,
    onProbe: vi.fn(),
    onControl: vi.fn(),
    onRateSync: vi.fn(),
    onManualPriority: vi.fn(),
    onEdit: vi.fn(),
    onDelete: vi.fn(),
  };
}

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  // JSDOM 26 recurses on these selectors; these menus never enter the top layer.
  const matches = Element.prototype.matches;
  vi.spyOn(Element.prototype, "matches").mockImplementation(function (
    this: Element,
    selector: string,
  ) {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("账号操作", () => {
  it("账号名称较长时，按钮名称和探活悬浮提示仅包含具体操作", async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider delay={0}>
        <AccountOperationButtons
          {...operationProps({ name: "上游平台的长名称账号-用于确认操作提示不追加账号名称" })}
        />
      </TooltipProvider>,
    );

    const actions = within(screen.getByRole("group", { name: "账号操作" }));
    for (const label of [
      "探活测试",
      "暂停调度",
      "手动熔断（停止调度）",
      "同步账号倍率",
      "设置人工优先位",
      "更多账号操作",
    ]) {
      expect(actions.getByRole("button", { name: label })).toBeVisible();
    }

    await user.hover(actions.getByRole("button", { name: "探活测试" }));

    expect(await screen.findByText("探活测试", { exact: true })).toBeVisible();
  });

  it.each([
    { name: "正常", overrides: {}, count: 6 },
    { name: "已排除", overrides: { health: "excluded", routing_state: "excluded" }, count: 5 },
    { name: "熔断", overrides: { health: "fused", routing_state: "fused" }, count: 6 },
    { name: "人工优先", overrides: { manual_priority: 3 }, count: 6 },
  ])("$name 账号仅展示具体操作，更多入口固定在三列两行的右下角", (fixture) => {
    render(<AccountOperationButtons {...operationProps(fixture.overrides)} />);

    const actions = screen.getByRole("group", { name: "账号操作" });
    expect(actions).toHaveClass("grid", "grid-cols-3");
    expect(within(actions).getAllByRole("button")).toHaveLength(fixture.count);
    expect(screen.getByRole("button", { name: "更多账号操作" })).toHaveClass(
      "col-start-3",
      "row-start-2",
    );
    expect(screen.queryByRole("button", { name: "状态与处置" })).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("键盘打开更多操作后可关闭并恢复焦点", async () => {
    const user = userEvent.setup();
    render(<AccountOperationButtons {...operationProps()} />);
    const more = screen.getByRole("button", { name: "更多账号操作" });
    more.focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("menu")).toBeVisible();
    expect(more).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("menuitem", { name: "删除账号及上游 Key" })).toBeVisible();
    expect(screen.queryByRole("menuitem", { name: "状态与处置" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(more).toHaveFocus());
    expect(more).toHaveAttribute("aria-expanded", "false");
  });

  it.each([
    { label: "查看并编辑账号", callback: "onEdit" as const },
    { label: "删除账号及上游 Key", callback: "onDelete" as const },
  ])("从更多菜单选择 $label 时进入对应操作并关闭菜单", async (fixture) => {
    const user = userEvent.setup();
    const props = operationProps();
    render(<AccountOperationButtons {...props} />);
    await user.click(screen.getByRole("button", { name: "更多账号操作" }));
    await user.click(await screen.findByRole("menuitem", { name: fixture.label }));
    expect(props[fixture.callback]).toHaveBeenCalledOnce();
    await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
  });

  it.each([
    { label: "探活测试", callback: "onProbe" as const },
    { label: "同步账号倍率", callback: "onRateSync" as const },
    { label: "设置人工优先位", callback: "onManualPriority" as const },
  ])("直接点击 $label 时进入对应操作", async (fixture) => {
    const user = userEvent.setup();
    const props = operationProps();
    render(<AccountOperationButtons {...props} />);
    await user.click(screen.getByRole("button", { name: fixture.label }));
    expect(props[fixture.callback]).toHaveBeenCalledOnce();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it.each([
    {
      name: "手动熔断（停止调度）",
      action: "fuse",
      label: "手动熔断",
      description: "直到手动解除",
      overrides: {},
    },
    {
      name: "解除熔断",
      action: "recover",
      label: "解除熔断",
      description: "仍受调度策略约束",
      overrides: { health: "fused", schedulable: false },
    },
    {
      name: "暂停调度",
      action: "pause",
      label: "暂停调度",
      description: "停止接收流量",
      overrides: {},
    },
  ])("直接点击 $name 时仍传递确认说明，不绕过既有确认流程", async (fixture) => {
    const user = userEvent.setup();
    const props = operationProps(fixture.overrides);
    render(<AccountOperationButtons {...props} />);
    await user.click(screen.getByRole("button", { name: fixture.name }));
    expect(props.onControl).toHaveBeenCalledWith(
      fixture.action,
      fixture.label,
      expect.stringContaining(fixture.description),
    );
  });

  it("账号操作进行中禁用所有操作及更多入口", () => {
    render(<AccountOperationButtons {...operationProps()} pending probePending />);
    expect(screen.getByRole("button", { name: "正在探活" })).toBeDisabled();
    for (const button of screen.getAllByRole("button")) expect(button).toBeDisabled();
  });

  it.each([false, true])(
    "人工优先账号的余额同步设置为 %s 时允许同步倍率和调整优先位，禁用自动处置",
    (syncBalance) => {
      render(
        <AccountOperationButtons
          {...operationProps({ manual_priority: 3, manual_sync_balance_multiplier: syncBalance })}
        />,
      );
      for (const label of ["探活测试", "暂停调度", "手动熔断（停止调度）"]) {
        expect(screen.getByRole("button", { name: label })).toBeDisabled();
      }
      expect(screen.getByRole("button", { name: "同步账号倍率" })).toBeEnabled();
      expect(screen.getByRole("button", { name: "调整人工优先位" })).toBeEnabled();
    },
  );

  it("暂停账号允许恢复调度并禁用手动熔断", async () => {
    const user = userEvent.setup();
    const props = operationProps({ health: "paused", routing_state: "paused", paused: true });
    render(<AccountOperationButtons {...props} />);
    expect(screen.getByRole("button", { name: "手动熔断（停止调度）" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "恢复调度" }));
    expect(props.onControl).toHaveBeenCalledWith("resume", "恢复调度", undefined);
  });

  it("策略已停止调度时不提供暂停和恢复调度操作", () => {
    render(
      <AccountOperationButtons
        {...operationProps({
          health: "cost_blocked",
          routing_state: "cost_blocked",
          schedulable: false,
        })}
      />,
    );
    expect(screen.queryByRole("button", { name: /暂停调度|恢复调度/ })).not.toBeInTheDocument();
  });

  it("已排除账号允许恢复管控，同步倍率仍可用且删除保留在菜单中", async () => {
    const user = userEvent.setup();
    const props = operationProps({
      health: "excluded",
      routing_state: "excluded",
      schedulable: false,
    });
    render(<AccountOperationButtons {...props} />);
    expect(screen.getByRole("button", { name: "同步账号倍率" })).toBeEnabled();
    expect(screen.queryByRole("button", { name: "删除账号及上游 Key" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "恢复管控" }));
    expect(props.onControl).toHaveBeenCalledWith("include", "恢复管控");
    await user.click(screen.getByRole("button", { name: "更多账号操作" }));
    expect(await screen.findByRole("menuitem", { name: "删除账号及上游 Key" })).toBeVisible();
  });
});
