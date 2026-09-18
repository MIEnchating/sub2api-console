import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { expect, it } from "vitest";
import { SuggestionInput } from "@/components/suggestion-input";

const models = ["gpt-5.5", "gpt-6-astra"];

function Field(props: { options?: string[]; disabled?: boolean }) {
  const [value, setValue] = useState("");
  return (
    <SuggestionInput
      aria-label="模型"
      value={value}
      onValueChange={setValue}
      options={props.options ?? models}
      disabled={props.disabled}
    />
  );
}

it("输入关键字时过滤建议，方向键和回车选择并关闭下拉", async () => {
  const user = userEvent.setup();
  render(<Field />);
  const input = screen.getByRole("combobox", { name: "模型" });
  await user.type(input, "astra");
  expect(screen.getByRole("option", { name: "gpt-6-astra" })).toBeVisible();
  expect(screen.queryByRole("option", { name: "gpt-5.5" })).not.toBeInTheDocument();
  expect(input).toHaveAttribute("aria-expanded", "true");
  await user.keyboard("{ArrowDown}{Enter}");
  expect(input).toHaveValue("gpt-6-astra");
  await waitFor(() => expect(input).toHaveAttribute("aria-expanded", "false"));
  expect(input).toHaveFocus();
});

it("输入自定义模型且没有匹配时保留输入，Escape 关闭后仍可继续编辑", async () => {
  const user = userEvent.setup();
  render(<Field />);
  const input = screen.getByRole("combobox", { name: "模型" });
  await user.type(input, "custom-model");
  expect(await screen.findByText("无匹配建议")).toBeVisible();
  await user.keyboard("{Escape}");
  expect(input).toHaveAttribute("aria-expanded", "false");
  expect(input).toHaveValue("custom-model");
  await user.type(input, "-v2");
  expect(input).toHaveValue("custom-model-v2");
});

it("建议为空时显示空状态，仍允许手动输入", async () => {
  const user = userEvent.setup();
  render(<Field options={[]} />);
  await user.click(screen.getByRole("button", { name: "展开建议" }));
  expect(await screen.findByText("暂无建议")).toBeVisible();
  const input = screen.getByRole("combobox", { name: "模型" });
  await user.type(input, "custom-model");
  expect(input).toHaveValue("custom-model");
});

it("禁用时输入和展开入口均不可操作", async () => {
  const user = userEvent.setup();
  render(<Field disabled />);
  const input = screen.getByRole("combobox", { name: "模型" });
  expect(input).toBeDisabled();
  expect(screen.getByRole("button", { name: "展开建议" })).toBeDisabled();
  await user.click(input);
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
});

it("长模型名按可用宽度换行，下拉跟随输入宽度并限制滚动高度", async () => {
  const user = userEvent.setup();
  const model = "long-model-".repeat(20);
  render(<Field options={[model]} />);
  await user.click(screen.getByRole("button", { name: "展开建议" }));
  expect(await screen.findByRole("option", { name: model })).toBeVisible();
  expect(screen.getByText(model)).toHaveClass("[overflow-wrap:anywhere]");
  const list = screen.getByRole("listbox");
  expect(list).toHaveClass("max-h-72", "overflow-y-auto");
  expect(list.closest('[data-slot="combobox-content"]')).toHaveClass(
    "bg-popover",
    "text-popover-foreground",
    "w-(--anchor-width)",
    "max-w-(--available-width)",
  );
});

it("外部更新和清空表单值时输入同步更新", () => {
  const view = render(
    <SuggestionInput
      aria-label="模型"
      options={models}
      value="gpt-5.5"
      onValueChange={() => undefined}
    />,
  );
  expect(screen.getByRole("combobox", { name: "模型" })).toHaveValue("gpt-5.5");
  view.rerender(
    <SuggestionInput aria-label="模型" options={models} value="" onValueChange={() => undefined} />,
  );
  expect(screen.getByRole("combobox", { name: "模型" })).toHaveValue("");
});

it("展开建议后保留显式标签名称，Tab 关闭下拉并聚焦下一操作", async () => {
  const user = userEvent.setup();
  render(
    <>
      <label id="model-label" htmlFor="model">
        检测模型
      </label>
      <SuggestionInput
        id="model"
        aria-labelledby="model-label"
        options={models}
        value=""
        onValueChange={() => undefined}
      />
      <button type="button">开始检测</button>
    </>,
  );
  await user.click(screen.getByRole("combobox", { name: "检测模型" }));
  expect(await screen.findByRole("option", { name: "gpt-5.5" })).toBeVisible();
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  await user.tab();
  expect(await screen.findByRole("button", { name: "开始检测" })).toHaveFocus();
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
});
