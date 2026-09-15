import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { expect, it, vi } from "vitest";

import { Input } from "@/components/ui/input";
import { PolicyConfigCard } from "../policy-config-card";

it.each([2, 3] as const)("配置 %s 列时，表单以单列起步并根据卡片宽度展开", (columns) => {
  render(
    <PolicyConfigCard title="健康分公式" description="按最新样本计算健康分。" columns={columns}>
      <Input aria-label="短期窗口" />
    </PolicyConfigCard>,
  );

  const card = screen.getByRole("region", { name: "健康分公式" });
  expect(card).toHaveClass("@container/policy-card", "min-w-0");
  const fields = card.querySelector('[data-slot="policy-fields"]');
  expect(fields).toHaveClass("grid-cols-1", "items-start", "@min-[28rem]/policy-card:grid-cols-2");
  if (columns === 3) expect(fields).toHaveClass("@min-[56rem]/policy-card:grid-cols-3");
  else expect(fields).not.toHaveClass("@min-[56rem]/policy-card:grid-cols-3");
});

function EnabledCard(): ReactElement {
  const [enabled, setEnabled] = useState(false);
  return (
    <PolicyConfigCard
      title="认证失效自动处置"
      description="账号反复出现认证失效时自动处置。"
      switchAction={{
        checked: enabled,
        label: "启用认证失效自动处置",
        onCheckedChange: setEnabled,
      }}
    >
      <Input aria-label="判定窗口" defaultValue="5" />
    </PolicyConfigCard>
  );
}

it("点击卡片标题后切换启用状态，同时保留已填写参数", async () => {
  const user = userEvent.setup();
  render(<EnabledCard />);
  await user.click(screen.getByText("认证失效自动处置", { exact: true }));

  expect(screen.getByRole("switch", { name: "启用认证失效自动处置" })).toBeChecked();
  expect(screen.getByText("已启用")).toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "判定窗口" })).toHaveValue("5");
});

it("聚焦卡片开关并按空格时，状态文字与开关同步", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const user = userEvent.setup();
  render(<EnabledCard />);
  const toggle = screen.getByRole("switch", { name: "启用认证失效自动处置" });
  toggle.focus();
  await user.keyboard(" ");

  expect(toggle).toBeChecked();
  expect(toggle).toHaveFocus();
  expect(screen.getByText("已启用")).toBeInTheDocument();
});

it("卡片开关禁用时点击标题不会提交变更，长标题仍可换行", async () => {
  const user = userEvent.setup();
  const onCheckedChange = vi.fn();
  const title = "认证失效自动处置".repeat(6);
  render(
    <PolicyConfigCard
      title={title}
      description="需要先读取当前配置。"
      wide
      switchAction={{ checked: false, disabled: true, label: "启用自动处置", onCheckedChange }}
    >
      <Input aria-label="判定窗口" />
    </PolicyConfigCard>,
  );
  await user.click(screen.getByText(title));

  expect(screen.getByRole("switch", { name: "启用自动处置" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(onCheckedChange).not.toHaveBeenCalled();
  expect(screen.getByRole("region", { name: title })).toHaveClass("xl:col-span-2");
  expect(screen.getByText(title)).toHaveClass("[overflow-wrap:anywhere]");
});
