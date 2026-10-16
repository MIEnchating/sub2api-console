import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { VerificationProbeNavigation } from "../verification-probe-navigation";
afterEach(cleanup);
it("桌面目录按类别分组，长标题单行截断且选中状态可访问", async () => {
  const select = vi.fn();
  const label = "知识边界 · " + "超长名称".repeat(30);
  render(
    <VerificationProbeNavigation
      probes={[
        { label: "多步计算", category: "capability", status: "passed" },
        { label, category: "public_reference", status: "error" },
      ]}
      selected={1}
      onSelect={select}
    />,
  );
  const nav = screen.getByRole("navigation", { name: "检测项" });
  expect(nav).toHaveClass("hidden", "md:block", "overflow-y-auto");
  expect(within(nav).getByText(label)).toHaveClass("truncate");
  const button = within(nav).getByRole("button", { name: /多步计算/ });
  button.focus();
  await userEvent.keyboard("{Enter}");
  expect(select).toHaveBeenCalledWith(0);
  expect(within(nav).getByRole("button", { name: /知识边界/ })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
});
it("空检测目录保留禁用选择器和空状态", () => {
  render(<VerificationProbeNavigation probes={[]} selected={0} onSelect={vi.fn()} />);
  expect(screen.getByRole("combobox", { name: "选择检测项" })).toBeDisabled();
  expect(
    within(screen.getByRole("navigation", { name: "检测项" })).getByText("没有逐项记录"),
  ).toBeVisible();
});

it("手机选择器切换检测项时按对应下标回调", async () => {
  const select = vi.fn();
  render(
    <VerificationProbeNavigation
      probes={[
        { label: "多步计算", status: "passed" },
        { label: "知识边界", status: "error" },
      ]}
      selected={0}
      onSelect={select}
    />,
  );
  await userEvent.click(screen.getByRole("combobox", { name: "选择检测项" }));
  await userEvent.click(await screen.findByRole("option", { name: /知识边界/ }));
  expect(select).toHaveBeenCalledWith(1);
});
