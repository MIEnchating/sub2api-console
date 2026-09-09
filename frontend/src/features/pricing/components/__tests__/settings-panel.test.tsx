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
    const description = screen.getByText(/均亏损时保留当前分组/);
    expect(description).not.toBeVisible();
    await user.click(screen.getByText("查看分组选择规则"));
    expect(description).toBeVisible();
  });
});
