import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render as renderComponent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { render } from "@/test/dictionary";
import { AccountStatusFilter } from "../account-status-tabs";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

it("字典为空时不回退本地选项", async () => {
  const user = userEvent.setup();
  render(<AccountStatusFilter value="all" onValueChange={vi.fn()} />, { account_status: [] });

  await user.click(screen.getByRole("button", { name: "状态筛选" }));

  expect(screen.queryByRole("option")).not.toBeInTheDocument();
  expect(screen.getByText("没有匹配项")).toBeVisible();
});

it("字典包含停用项及未实现的状态时，仅展示启用且受支持的选项和字典名称", async () => {
  const user = userEvent.setup();
  const view = render(<AccountStatusFilter value="all" onValueChange={vi.fn()} />, {});
  act(() => {
    view.client.setQueryData(["dictionaries", "account_status"], {
      items: [
        { value: "healthy", name: "健康状态", enabled: true },
        { value: "manual_fused", name: "人工熔断", enabled: false },
        { value: "future_status", name: "未实现状态", enabled: true },
      ],
    });
  });

  await user.click(screen.getByRole("button", { name: "状态筛选" }));

  expect(screen.getAllByRole("option")).toHaveLength(1);
  expect(screen.getByRole("option", { name: "健康状态" })).toBeVisible();
});

it("字典缓存增加人工熔断并重排后，打开的筛选同步更新", async () => {
  const user = userEvent.setup();
  const view = render(<AccountStatusFilter value="all" onValueChange={vi.fn()} />, {
    account_status: [{ value: "healthy", name: "健康" }],
  });
  await user.click(screen.getByRole("button", { name: "状态筛选" }));
  expect(screen.queryByRole("option", { name: "人工熔断" })).not.toBeInTheDocument();

  act(() => {
    view.client.setQueryData(["dictionaries", "account_status"], {
      items: [
        { value: "manual_fused", name: "人工熔断", enabled: true },
        { value: "healthy", name: "健康", enabled: true },
      ],
    });
  });

  await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(2));
  expect(screen.getAllByRole("option")[0]).toHaveAccessibleName("人工熔断");
});

it("已选状态从字典消失时保留筛选回显和清除入口，但不补入可选项", async () => {
  const user = userEvent.setup();
  const onValueChange = vi.fn();
  render(<AccountStatusFilter value="manual_fused" onValueChange={onValueChange} />, {
    account_status: [{ value: "healthy", name: "健康" }],
  });
  expect(screen.getByRole("button", { name: "状态筛选" })).toHaveTextContent("人工熔断");
  await user.click(screen.getByRole("button", { name: "状态筛选" }));
  expect(screen.queryByRole("option", { name: "人工熔断" })).not.toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "清除筛选" }));

  expect(onValueChange).toHaveBeenCalledWith("all");
});

it("字典首次读取期间显示加载提示且菜单仍可关闭", async () => {
  const user = userEvent.setup();
  render(<AccountStatusFilter value="all" onValueChange={vi.fn()} />, {});
  await user.click(screen.getByRole("button", { name: "状态筛选" }));

  expect(screen.getByRole("status", { name: "正在读取账号状态" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  expect(screen.queryByRole("option")).not.toBeInTheDocument();
  expect(screen.queryByText("没有匹配项")).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  expect(screen.getByRole("button", { name: "状态筛选" })).toHaveFocus();
});

it("字典请求失败时不补入本地选项，重试成功后展示服务器状态", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockRejectedValueOnce(new TypeError("网络不可用"))
      .mockResolvedValueOnce(
        Response.json({ items: [{ value: "healthy", name: "健康", enabled: true }] }),
      ),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  renderComponent(
    <QueryClientProvider client={client}>
      <AccountStatusFilter value="all" onValueChange={vi.fn()} />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "状态筛选" }));
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByRole("option")).not.toBeInTheDocument();

  await user.click(retry);

  expect(await screen.findByRole("option", { name: "健康" })).toBeVisible();
  expect(screen.getAllByRole("option")).toHaveLength(1);
});

it("后台字典刷新失败时保留已读取的选项", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("网络不可用")));
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["dictionaries", "account_status"], {
    items: [{ value: "healthy", name: "健康", enabled: true }],
  });
  renderComponent(
    <QueryClientProvider client={client}>
      <AccountStatusFilter value="all" onValueChange={vi.fn()} />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "状态筛选" }));

  await act(async () => {
    await client.invalidateQueries({ queryKey: ["dictionaries", "account_status"] });
  });

  expect(screen.getAllByRole("option")).toHaveLength(1);
  expect(screen.getByRole("option", { name: "健康" })).toBeVisible();
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});
