import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { expect, it } from "vitest";
import { DetectionModelSelect } from "../detection-model-select";

it("正在提交时模型下拉框禁用且不能展开", async () => {
  render(
    <DetectionModelSelect
      model="first"
      additionalModels={["second"]}
      onChange={() => undefined}
      disabled
    />,
  );
  const trigger = screen.getByRole("combobox", { name: "检测模型" });
  expect(trigger).toHaveAttribute("aria-disabled", "true");
  await userEvent.setup().click(trigger);
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByRole("option")).not.toBeInTheDocument();
});
function FullSelection(): ReactElement {
  const [models, setModels] = useState(Array.from({ length: 16 }, (_, i) => `m-${i}`));
  return (
    <DetectionModelSelect
      model={models[0]}
      additionalModels={models.slice(1)}
      options={["another-model"]}
      onChange={(model, additional) => setModels([model, ...additional])}
      disabled={false}
    />
  );
}
it("达到16个模型时禁止继续勾选但允许取消已有选择", async () => {
  render(<FullSelection />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("combobox", { name: "检测模型" }));
  expect(screen.getByRole("option", { name: "another-model" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  await user.click(screen.getByRole("option", { name: "m-0" }));
  expect(screen.getByRole("option", { name: "another-model" })).not.toHaveAttribute(
    "aria-disabled",
    "true",
  );
});
it("模型校验失败时下拉框关联可见错误提示", () => {
  render(
    <>
      <DetectionModelSelect
        model=""
        onChange={() => undefined}
        disabled={false}
        invalid
        describedBy="model-error"
      />
      <p id="model-error">请选择检测模型</p>
    </>,
  );
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveAccessibleDescription(
    "请选择检测模型",
  );
});

it("没有模型时提示手动添加，超长 ID 不可添加且保留错误原因", async () => {
  render(<DetectionModelSelect model="" onChange={() => undefined} disabled={false} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("combobox", { name: "检测模型" }));
  expect(await screen.findByText("输入模型 ID 添加，或先获取模型列表")).toBeVisible();
  await user.click(screen.getByRole("combobox", { name: "搜索或输入模型 ID" }));
  await user.paste("a".repeat(257));
  expect(screen.queryByRole("option")).not.toBeInTheDocument();
  expect(screen.getByText("模型 ID 不能超过 256 个字符或包含控制字符")).toBeVisible();
});

it("选中两个模型时直接显示两个名称，第三个起才折叠数量", () => {
  const view = render(
    <DetectionModelSelect
      model="gpt-6-astra"
      additionalModels={["gpt-6.1-sol"]}
      disabled={false}
      onChange={() => undefined}
    />,
  );
  const trigger = screen.getByRole("combobox", { name: "检测模型" });
  expect(trigger).toHaveTextContent("gpt-6-astra");
  expect(trigger).toHaveTextContent("gpt-6.1-sol");
  expect(trigger).not.toHaveTextContent("+1");
  view.rerender(
    <DetectionModelSelect
      model="gpt-6-astra"
      additionalModels={["gpt-6.1-sol", "third"]}
      disabled={false}
      onChange={() => undefined}
    />,
  );
  expect(trigger).toHaveTextContent("+1");
});

it("下拉已展开时再次点击选择框保持关闭，下一次点击才展开", async () => {
  render(<DetectionModelSelect model="gpt-6-astra" disabled={false} onChange={() => undefined} />);
  const user = userEvent.setup();
  const trigger = screen.getByRole("combobox", { name: "检测模型" });
  await user.click(trigger);
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  await user.click(trigger);
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  await user.click(trigger);
  expect(trigger).toHaveAttribute("aria-expanded", "true");
});

it("键盘展开模型选择后聚焦搜索框，关闭后焦点返回触发按钮", async () => {
  render(<DetectionModelSelect model="gpt-6-astra" disabled={false} onChange={() => undefined} />);
  const user = userEvent.setup();
  await user.tab();
  const trigger = screen.getByRole("combobox", { name: "检测模型" });
  expect(trigger).toHaveFocus();
  await user.keyboard("{ArrowDown}");
  const search = screen.getByRole("combobox", { name: "搜索或输入模型 ID" });
  expect(search).toHaveFocus();
  await user.keyboard("{Escape}");
  expect(trigger).toHaveFocus();
});
