import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { FormField } from "@/components/form-field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

describe("共享字段标签", () => {
  it("复合字段指定 htmlFor 时标签只关联目标输入框，保留选择器独立名称", () => {
    render(
      <FormField label="上游地址" htmlFor="upstream-address">
        <Select defaultValue="https">
          <SelectTrigger aria-label="上游地址协议">
            <SelectValue />
          </SelectTrigger>
        </Select>
        <Input id="upstream-address" />
      </FormField>,
    );
    expect(screen.getByRole("combobox", { name: "上游地址协议" })).toBeEnabled();
    expect(screen.getByLabelText("上游地址", { exact: true })).toBe(screen.getByRole("textbox"));
  });
  it("省略 htmlFor 时仍为输入框提供名称，点击标签聚焦输入框", async () => {
    render(
      <FormField label="上游地址">
        <Input />
      </FormField>,
    );
    const input = screen.getByRole("textbox", { name: "上游地址" });
    await userEvent.setup().click(screen.getByText("上游地址", { exact: true }));
    expect(input).toHaveFocus();
  });

  it("选择器嵌套在字段中时继承名称并可用键盘选择", async () => {
    render(
      <FormField label="平台">
        <Select defaultValue="sub2api">
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent searchable={false}>
            <SelectItem value="sub2api">Sub2API</SelectItem>
          </SelectContent>
        </Select>
      </FormField>,
    );
    const select = screen.getByRole("combobox", { name: /平台/ });
    expect(select).toHaveAttribute("aria-expanded", "false");
    select.focus();
    await userEvent.setup().keyboard("{ArrowDown}");
    expect(select).toHaveAttribute("aria-expanded", "true");
  });
});
