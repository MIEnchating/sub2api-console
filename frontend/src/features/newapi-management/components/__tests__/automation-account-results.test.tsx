import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { Task } from "@/api";
import { AccountModelAutomationResults } from "../account-model-automation-results";

function task(failed = true): Task {
  return {
    id: "account-results",
    skill: "console",
    operation: "account-model-automation",
    status: failed ? "failed" : "succeeded",
    progress: 100,
    message: "模型目录处理结束",
    created_at: "2026-10-12T00:00:00Z",
    updated_at: "2026-10-12T00:00:00Z",
    result: {
      dry_run: false,
      result_version: 2,
      items: [
        ...Array.from({ length: 21 }, (_, i) => ({
          account_id: String(i + 1),
          status: "succeeded",
          models: ["model-a"],
        })),
        ...(failed
          ? [{ account_id: "22", status: "failed", message: "管理接口请求超时，请重试" }]
          : []),
      ],
    },
  };
}

it("失败账号在原始结果末尾时默认展示失败明细，切换全部可查看成功项", async () => {
  render(<AccountModelAutomationResults task={task()} />);
  const region = screen.getByRole("region", { name: "账号模型目录" });
  expect(within(region).getByRole("button", { name: "失败 1" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  expect(within(region).getAllByRole("listitem")).toHaveLength(1);
  expect(within(region).getByText("管理接口请求超时，请重试")).toBeVisible();
  expect(within(region).getByText("账号 22 · 目录同步失败")).toBeVisible();
  await userEvent.click(within(region).getByRole("button", { name: "全部 22" }));
  expect(within(region).getAllByRole("listitem")).toHaveLength(20);
  expect(within(region).getByText("账号 1 · 目录已刷新")).toBeVisible();
  await userEvent.click(within(region).getByRole("button", { name: "转到下一页" }));
  within(region).getByRole("button", { name: "成功 21" }).focus();
  await userEvent.keyboard("{Enter}");
  expect(within(region).getByText("账号 1 · 目录已刷新")).toBeVisible();
});

it("成功结果切换失败筛选时展示空状态，模型列表可展开", async () => {
  render(<AccountModelAutomationResults task={task(false)} />);
  const region = screen.getByRole("region", { name: "账号模型目录" });
  expect(within(region).getByRole("list")).toHaveClass("max-h-96", "overflow-y-auto");
  const summary = within(region).getAllByText("查看账号可用模型（1 个）")[0];
  await userEvent.click(summary);
  expect(summary.closest("details")).toHaveAttribute("open");
  await userEvent.click(within(region).getByRole("button", { name: "失败 0" }));
  expect(within(region).getByText("没有失败的账号")).toBeVisible();
});

it("账号结果多页时使用紧凑分页，范围说明合并并允许窄屏换行", () => {
  const result = task(false);
  result.result.pricing_enabled = false;
  render(<AccountModelAutomationResults task={result} />);
  const region = screen.getByRole("region", { name: "账号模型目录" });
  const pagination = within(region).getByRole("navigation", { name: "表格分页" });
  expect(within(pagination).getAllByRole("button")).toHaveLength(2);
  expect(within(pagination).getByLabelText("当前第 1 页，共 2 页")).toBeVisible();
  const notes = screen.getByRole("list", { name: "执行范围说明" });
  expect(notes).toHaveClass("flex", "flex-wrap", "min-w-0");
  expect(within(notes).getAllByRole("listitem")).toHaveLength(2);
  expect(within(notes).getByText("本次没有渠道上架记录。")).toBeVisible();
  expect(within(notes).getByText("本次不修改模型价格。")).toBeVisible();
});

it("后台刷新新增失败项时自动展示原因，但保留用户主动选择的筛选", async () => {
  const view = render(<AccountModelAutomationResults task={task(false)} />);
  view.rerender(<AccountModelAutomationResults task={task()} />);
  expect(screen.getByText("管理接口请求超时，请重试")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "成功 21" }));
  view.rerender(<AccountModelAutomationResults task={task()} />);
  expect(screen.getByRole("button", { name: "成功 21" })).toHaveAttribute("aria-pressed", "true");
});
