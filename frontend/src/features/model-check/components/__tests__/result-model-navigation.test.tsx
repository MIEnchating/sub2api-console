import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { expect, it } from "vitest";
import { ResultModelSelector } from "../result-model-selector";

function Results(): ReactElement {
  const [model, setModel] = useState("gpt-6-astra");
  return (
    <ResultModelSelector
      models={["gpt-6-astra", "gpt-6.1-sol", "third-model"]}
      value={model}
      onChange={setModel}
    />
  );
}

it("多个模型用左右按钮切换，首尾按钮禁用并显示当前序号", async () => {
  render(<Results />);
  const user = userEvent.setup();
  const navigation = within(screen.getByRole("group", { name: "结果模型" }));
  expect(navigation.getByText("gpt-6-astra")).toBeVisible();
  expect(navigation.getByText("1 / 3")).toBeVisible();
  expect(navigation.getByRole("button", { name: "上一个模型结果" })).toBeDisabled();
  await user.click(navigation.getByRole("button", { name: "下一个模型结果" }));
  expect(navigation.getByText("gpt-6.1-sol")).toBeVisible();
  expect(navigation.getByText("2 / 3")).toBeVisible();
  await user.click(navigation.getByRole("button", { name: "下一个模型结果" }));
  expect(navigation.getByRole("button", { name: "下一个模型结果" })).toBeDisabled();
  await user.click(navigation.getByRole("button", { name: "上一个模型结果" }));
  expect(navigation.getByText("gpt-6.1-sol")).toBeVisible();
});

it("聚焦切换栏后方向键及首尾键切换结果且不越界", async () => {
  render(<Results />);
  const user = userEvent.setup();
  const navigation = screen.getByRole("group", { name: "结果模型" });
  navigation.focus();
  await user.keyboard("{ArrowRight}");
  expect(navigation).toHaveTextContent("gpt-6.1-sol");
  await user.keyboard("{End}{ArrowRight}");
  expect(navigation).toHaveTextContent("third-model");
  await user.keyboard("{Home}{ArrowLeft}");
  expect(navigation).toHaveTextContent("gpt-6-astra");
});

it.each([undefined, [], ["only-model"]])("空结果或单模型不显示切换栏：%j", (models) => {
  render(<ResultModelSelector models={models} onChange={() => undefined} />);
  expect(screen.queryByRole("group", { name: "结果模型" })).not.toBeInTheDocument();
});

it("当前模型被移除时回到第一个，悬停长模型名称可查看完整内容", async () => {
  const model = "long-model-".repeat(20);
  const user = userEvent.setup();
  const view = render(
    <ResultModelSelector models={["first", model]} value={model} onChange={() => undefined} />,
  );
  await user.hover(screen.getByText(model));
  expect(await screen.findByRole("tooltip")).toHaveTextContent(model);
  view.rerender(
    <ResultModelSelector models={["first", "second"]} value={model} onChange={() => undefined} />,
  );
  expect(screen.getByRole("group", { name: "结果模型" })).toHaveTextContent("first");
  expect(screen.getByText("1 / 2")).toBeVisible();
});
