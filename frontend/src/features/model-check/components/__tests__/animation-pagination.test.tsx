import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(count: number): { dispose: () => void } {
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(
    ["accounts"],
    Array.from({ length: count }, (_, index) => ({
      id: String(index + 1),
      name: `分页账号 ${index + 1}`,
      groups: [],
      platform: "openai",
    })),
  );
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], []);
  const view = render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  return {
    dispose: () => {
      view.unmount();
      client.clear();
    },
  };
}

it("账号较多时默认只挂载 12 张卡片，通过分页访问剩余账号", async () => {
  const view = setup(60);
  expect(screen.getAllByRole("checkbox", { name: /检测 分页账号/ })).toHaveLength(12);
  expect(screen.queryByRole("checkbox", { name: /^检测 分页账号 13\b/ })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(await screen.findByRole("checkbox", { name: /^检测 分页账号 13\b/ })).toBeVisible();
  expect(screen.getAllByRole("checkbox", { name: /检测 分页账号/ })).toHaveLength(12);
  view.dispose();
});

it("跨页勾选和搜索后保留统一模型，并提交完整的已选范围", async () => {
  const user = userEvent.setup();
  const view = setup(25);
  await user.click(screen.getByRole("checkbox", { name: /^检测 分页账号 1\b/ }));
  await user.type(screen.getByRole("combobox", { name: "检测模型" }), "first-model");
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  await user.click(screen.getByRole("checkbox", { name: /^检测 分页账号 13\b/ }));
  const search = screen.getByRole("textbox", { name: "搜索动画检测账号" });
  await user.type(search, "分页账号 25");
  expect(await screen.findByRole("checkbox", { name: /^检测 分页账号 25\b/ })).toBeVisible();
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
  await user.clear(search);
  expect(await screen.findByRole("checkbox", { name: /^检测 分页账号 1\b/ })).toBeChecked();
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveValue("first-model");
  await user.click(screen.getByRole("button", { name: "开始检测（2 个账号）" }));
  const confirmation = await screen.findByRole("dialog", { name: "确认动画检测范围" });
  expect(confirmation).toHaveTextContent("ID 1）→ first-model");
  expect(confirmation).toHaveTextContent("ID 13）→ first-model");
  await user.click(within(confirmation).getByRole("button", { name: "取消" }));
  view.dispose();
});

it("选择前 20 个账号覆盖分页范围，到达上限后禁用未选账号并允许清空", async () => {
  const view = setup(25);
  fireEvent.click(screen.getByRole("button", { name: "选择前 20 个账号" }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "开始检测（20 个账号）" })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByRole("checkbox", { name: /^检测 分页账号 20\b/ })).toBeChecked();
  expect(screen.getByRole("checkbox", { name: /^检测 分页账号 21\b/ })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  fireEvent.click(screen.getByRole("button", { name: "清空选择" }));
  expect(screen.getByRole("checkbox", { name: /^检测 分页账号 21\b/ })).not.toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByRole("button", { name: "开始检测（0 个账号）" })).toBeDisabled();
  view.dispose();
});

it("翻页后提交缺少统一模型时，聚焦顶部模型字段", async () => {
  const view = setup(25);
  fireEvent.click(screen.getByRole("checkbox", { name: /^检测 分页账号 1\b/ }));
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  fireEvent.click(screen.getByRole("button", { name: "开始检测（1 个账号）" }));
  const model = screen.getByRole("combobox", { name: "检测模型" });
  await waitFor(() => expect(model).toHaveFocus());
  expect(model).toHaveAttribute("aria-invalid", "true");
  expect(screen.getByText("请输入模型 ID")).toBeVisible();
  view.dispose();
});
