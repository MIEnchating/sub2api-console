import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import { render } from "@/test/dictionary";
import { AccountStatusFilter } from "../account-status-tabs";
import { accountStatusDictionary } from "./status-dictionary-fixture";

it("字典未登记人工熔断时，列表和搜索均不补入前端状态", async () => {
  const user = userEvent.setup();
  render(<AccountStatusFilter value="all" onValueChange={vi.fn()} />, {
    account_status: [{ value: "healthy", name: "健康" }],
  });
  await user.click(screen.getByRole("button", { name: "状态筛选" }));

  expect(screen.getAllByRole("option")).toHaveLength(1);
  expect(screen.queryByRole("option", { name: "人工熔断" })).not.toBeInTheDocument();
  await user.type(screen.getByRole("combobox", { name: "搜索状态" }), "人工熔断");
  expect(screen.queryByRole("option")).not.toBeInTheDocument();
});

it("默认展示全部十二个状态，并按字典顺序支持键盘选择", async () => {
  const user = userEvent.setup();
  const onValueChange = vi.fn();
  render(<AccountStatusFilter value="all" onValueChange={onValueChange} />, {
    account_status: accountStatusDictionary,
  });
  await user.click(screen.getByRole("button", { name: "状态筛选" }));

  const options = screen.getAllByRole("option");
  expect(options).toHaveLength(12);
  expect(options[0]).toHaveAccessibleName("人工熔断");
  expect(options[1]).toHaveAccessibleName("降级");
  expect(screen.queryByRole("button", { name: "更多状态" })).not.toBeInTheDocument();
  await user.keyboard("{Home}{Enter}");
  expect(onValueChange).toHaveBeenCalledWith("manual_fused");
});

it("搜索状态名称时缩小列表，并可用键盘选择已排除", async () => {
  const user = userEvent.setup();
  const onValueChange = vi.fn();
  render(<AccountStatusFilter value="all" onValueChange={onValueChange} />, {
    account_status: accountStatusDictionary,
  });
  await user.click(screen.getByRole("button", { name: "状态筛选" }));
  expect(screen.getByRole("option", { name: "已排除" })).toBeVisible();

  await user.type(screen.getByRole("combobox", { name: "搜索状态" }), "已排除");

  expect(screen.getAllByRole("option")).toHaveLength(1);
  expect(screen.getByRole("option", { name: "已排除" })).toBeEnabled();
  await user.keyboard("{Enter}");
  expect(onValueChange).toHaveBeenCalledWith("excluded");
});

it("直接选择已停用后保留完整列表和选中状态，清除后恢复全部账号", async () => {
  const user = userEvent.setup();
  const onValueChange = vi.fn();
  const view = render(<AccountStatusFilter value="all" onValueChange={onValueChange} />, {
    account_status: accountStatusDictionary,
  });
  await user.click(screen.getByRole("button", { name: "状态筛选" }));
  expect(screen.getAllByRole("option")).toHaveLength(12);
  await user.click(screen.getByRole("option", { name: "已停用" }));
  expect(onValueChange).toHaveBeenCalledWith("disabled");
  view.rerender(<AccountStatusFilter value="disabled" onValueChange={onValueChange} />);

  expect(screen.getAllByRole("option")).toHaveLength(12);
  expect(screen.getByRole("option", { name: "已停用" })).toHaveAttribute("aria-selected", "true");
  await user.click(screen.getByRole("button", { name: "清除筛选" }));
  expect(onValueChange).toHaveBeenCalledWith("all");
});

it("搜索无匹配时显示空提示，清空搜索后恢复全部状态", async () => {
  const user = userEvent.setup();
  render(<AccountStatusFilter value="all" onValueChange={vi.fn()} />, {
    account_status: accountStatusDictionary,
  });
  await user.click(screen.getByRole("button", { name: "状态筛选" }));
  const search = screen.getByRole("combobox", { name: "搜索状态" });
  await user.type(search, "不存在的状态");
  expect(screen.queryByRole("option")).not.toBeInTheDocument();
  expect(screen.getByText("没有匹配项")).toBeVisible();

  await user.clear(search);

  expect(screen.getAllByRole("option")).toHaveLength(12);
  expect(screen.queryByRole("button", { name: "更多状态" })).not.toBeInTheDocument();
});

it("搜索协议值后关闭菜单，重新打开时清空搜索并恢复全部状态", async () => {
  const user = userEvent.setup();
  render(<AccountStatusFilter value="all" onValueChange={vi.fn()} />, {
    account_status: accountStatusDictionary,
  });
  await user.click(screen.getByRole("button", { name: "状态筛选" }));
  await user.type(screen.getByRole("combobox", { name: "搜索状态" }), "concurrency_limited");
  expect(screen.getAllByRole("option")).toHaveLength(1);
  expect(screen.getByRole("option", { name: "等待并发额度" })).toBeVisible();

  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("button", { name: "状态筛选" }));

  expect(screen.getByRole("combobox", { name: "搜索状态" })).toHaveValue("");
  expect(screen.getAllByRole("option")).toHaveLength(12);
});
