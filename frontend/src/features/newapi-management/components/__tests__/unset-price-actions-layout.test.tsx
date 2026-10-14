import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import { NewAPIModelPrices } from "../model-prices";

it("未设置模型同时提供查询和套用时，操作按钮右对齐并保留固定间距", async () => {
  const user = userEvent.setup();
  render(
    <NewAPIModelPrices
      models={[{ model: "source", input_ratio: "1", completion_ratio: "5" }]}
      unsetModels={[{ model: "gemini-4-argon", input_ratio: "", completion_ratio: "" }]}
      onWriteModelPrice={vi.fn().mockResolvedValue(true)}
    />,
  );

  await user.click(screen.getByRole("tab", { name: "未设置模型价格" }));
  const actions = screen.getByRole("group", { name: "gemini-4-argon 价格操作" });
  expect(actions).toHaveClass("flex", "items-center", "justify-end", "gap-1");
  const query = within(actions).getByRole("button", { name: "查询 gemini-4-argon 远程模型价格" });
  const apply = within(actions).getByRole("button", { name: "套用其他模型价格到 gemini-4-argon" });
  expect(query).toBeEnabled();
  expect(apply).toBeEnabled();
  query.focus();
  await user.tab();
  expect(apply).toHaveFocus();
});

it("没有可套用价格时，操作组保留右对齐的查询按钮", async () => {
  const user = userEvent.setup();
  render(
    <NewAPIModelPrices
      models={[]}
      unsetModels={[{ model: "gemini-4-argon", input_ratio: "", completion_ratio: "" }]}
    />,
  );

  await user.click(screen.getByRole("tab", { name: "未设置模型价格" }));
  const actions = screen.getByRole("group", { name: "gemini-4-argon 价格操作" });
  expect(actions).toHaveClass("flex", "justify-end", "gap-1");
  expect(within(actions).getAllByRole("button")).toHaveLength(1);
  expect(
    within(actions).getByRole("button", { name: "查询 gemini-4-argon 远程模型价格" }),
  ).toBeEnabled();
});
