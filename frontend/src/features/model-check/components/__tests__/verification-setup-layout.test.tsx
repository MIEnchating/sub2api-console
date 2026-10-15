import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { VerificationSetup } from "../verification-setup";

it("指纹选项将勾选框与标题居中对齐且窄屏说明独立换行", () => {
  render(<VerificationSetup fingerprint disabled={false} onFingerprintChange={vi.fn()} />);

  const checkbox = screen.getByRole("checkbox", { name: "采集分词指纹" });
  const label = screen.getByText("采集分词指纹", { selector: "label" });
  expect(label).toContainElement(checkbox);
  expect(label).toHaveClass("inline-flex", "items-center", "shrink-0");
  expect(label.parentElement).toHaveClass("flex", "flex-col", "sm:flex-row", "sm:items-center");
  expect(checkbox).toHaveAccessibleDescription(/每个组合额外 10 次请求/);
});

it("点击指纹标题或按空格可以切换选项", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const onChange = vi.fn();
  render(<VerificationSetup fingerprint disabled={false} onFingerprintChange={onChange} />);
  const user = userEvent.setup();
  await user.click(screen.getByText("采集分词指纹", { selector: "label" }));
  expect(onChange).toHaveBeenLastCalledWith(false, expect.anything());
  onChange.mockClear();
  screen.getByRole("checkbox", { name: "采集分词指纹" }).focus();
  await user.keyboard(" ");
  expect(onChange).toHaveBeenLastCalledWith(false, expect.anything());
});

it("检测期间点击指纹标题不会改变禁用选项", async () => {
  const onChange = vi.fn();
  render(<VerificationSetup fingerprint disabled onFingerprintChange={onChange} />);
  await userEvent.setup().click(screen.getByText("采集分词指纹", { selector: "label" }));
  expect(screen.getByRole("checkbox", { name: "采集分词指纹" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(onChange).not.toHaveBeenCalled();
});
