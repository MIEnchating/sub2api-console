import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PricingConfigDraft } from "../../types";
import { PricingSettingsPanel } from "../pricing-settings-panel";

function SettingsFixture(): ReactElement {
  const [value, setValue] = useState<PricingConfigDraft>({
    enabled: false,
    profit_margin: 0.2,
    interval_seconds: 120,
    write_concurrency: 4,
    exchange_group_sets: [],
    exchange_group_set_names: [],
  });
  return <PricingSettingsPanel value={value} onChange={setValue} />;
}

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

describe("价格设置卡", () => {
  it("参数区域切换横排或侧栏时使用单层间距并保持输入对齐", () => {
    render(<SettingsFixture />);

    const settings = screen.getByTestId("pricing-settings-grid");
    expect(settings).toHaveClass("lg:grid-cols-3", "xl:grid-cols-1");
    expect(settings).not.toHaveClass("group-data-[size=sm]/card:px-3");
    expect(settings).not.toHaveClass("group-data-[size=sm]/card:py-3");
    expect(screen.getByTestId("pricing-goal-settings")).toHaveClass(
      "grid-cols-[minmax(0,1fr)_8rem]",
      "lg:grid-cols-1",
    );
    expect(screen.getByTestId("pricing-execution-settings")).toHaveClass(
      "grid-cols-[minmax(0,1fr)_8rem]",
      "lg:grid-cols-1",
    );
  });

  it("自动执行开关显示关联标签，键盘切换后可访问状态与徽标同步", async () => {
    const user = userEvent.setup();
    render(<SettingsFixture />);
    const toggle = screen.getByRole("switch", { name: "启用动态价格分组" });

    expect(screen.getByText("启用动态价格分组", { selector: "label" })).toBeVisible();
    expect(toggle).not.toBeChecked();
    expect(toggle).toHaveAccessibleDescription("默认关闭");

    await user.tab();
    expect(toggle).toHaveFocus();
    await user.keyboard(" ");

    expect(toggle).toBeChecked();
    expect(toggle).toHaveAccessibleDescription("已开启");
    expect(screen.getByText("已开启")).toBeVisible();
  });

  it("点击自动执行的可见标签时切换对应开关", async () => {
    const user = userEvent.setup();
    render(<SettingsFixture />);

    await user.click(screen.getByText("启用动态价格分组", { selector: "label" }));

    expect(screen.getByRole("switch", { name: "启用动态价格分组" })).toBeChecked();
  });

  it("数字参数清空后允许重新输入，切换自动调整不丢失参数", async () => {
    const user = userEvent.setup();
    render(<SettingsFixture />);
    const margin = screen.getByRole("spinbutton", { name: "目标盈利比例" });
    await user.clear(margin);
    expect(margin).toHaveValue(null);
    await user.type(margin, "25");
    await user.click(screen.getByRole("switch", { name: "启用动态价格分组" }));
    expect(screen.getByText("已开启")).toBeVisible();
    expect(margin).toHaveValue(25);
    expect(screen.getByRole("spinbutton", { name: "动态调整间隔" })).toHaveValue(120);
    expect(screen.getByRole("spinbutton", { name: "写入并发" })).toHaveValue(4);
  });

  it("详细规则默认折叠，点击展开后可读取亏损回退说明", async () => {
    const user = userEvent.setup();
    render(<SettingsFixture />);
    const description = screen.getByText(/无合适分组时保留当前分组/);
    expect(description).not.toBeVisible();
    await user.click(screen.getByText("查看分组选择规则"));
    expect(description).toBeVisible();
  });
});
