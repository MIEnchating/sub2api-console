import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import { createConsoleQueryClient } from "@/lib/query-client";
import { ImportPanel } from "../components/import-panel";
import { templateKeys } from "../constants";
import { template } from "./template-fixture";

const clients: ReturnType<typeof createConsoleQueryClient>[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
});

function mount(): ReturnType<typeof createConsoleQueryClient> {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <ImportPanel onStarted={() => undefined} />
      <Toaster />
    </QueryClientProvider>,
  );
  return client;
}

it("模板首次加载显示忙碌提示，切换独立输出取消读取并保留账号资料", async () => {
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  const fetcher = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  vi.stubGlobal("fetch", fetcher);
  const client = mount();
  expect(screen.getByRole("status", { name: "正在读取配置模板" })).toBeVisible();
  expect(screen.getByRole("combobox", { name: "配置模板" })).toBeDisabled();
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_isolated");
  await user.click(screen.getByRole("combobox", { name: "处理方式" }));
  await user.click(await screen.findByRole("option", { name: "独立 JSON 输出" }));
  await act(async () => finish(Response.json({ revision: 0, preferred_id: "", items: [] })));
  expect(signal?.aborted).toBe(true);
  expect(client.getQueryData(templateKeys.library)).toBeUndefined();
  expect(screen.queryByRole("status", { name: "正在读取配置模板" })).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "账号资料" })).toHaveValue("rt_isolated");
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeEnabled();
  expect(fetcher).toHaveBeenCalledOnce();
});

it("模板首次读取失败仅通知一次，重试成功保留输入并恢复模板选择", async () => {
  let finish!: (response: Response) => void;
  let reads = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(() => {
      reads++;
      if (reads === 1)
        return Promise.resolve(Response.json({ detail: "模板暂不可用" }, { status: 503 }));
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }),
  );
  mount();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getAllByText("模板暂不可用")).toHaveLength(1);
  expect(screen.getByRole("combobox", { name: "配置模板" })).toBeDisabled();
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_isolated");
  await user.click(retry);
  expect(await screen.findByRole("status", { name: "正在读取配置模板" })).toBeVisible();
  expect(screen.getByRole("combobox", { name: "配置模板" })).toBeDisabled();
  await act(async () =>
    finish(Response.json({ revision: 1, preferred_id: "", items: [template] })),
  );
  await waitFor(() => expect(screen.getByRole("combobox", { name: "配置模板" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "配置模板" }));
  await user.click(await screen.findByRole("option", { name: template.name }));
  expect(screen.getByRole("combobox", { name: "配置模板" })).toHaveTextContent(template.name);
  expect(screen.getByRole("textbox", { name: "账号资料" })).toHaveValue("rt_isolated");
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});

it("模板后台刷新失败保留已选模板，独立输出往返后仍保留选择", async () => {
  let fail = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      fail
        ? Response.json({ detail: "模板刷新失败" }, { status: 503 })
        : Response.json({ revision: 1, preferred_id: "", items: [template] }),
    ),
  );
  const client = mount();
  const user = userEvent.setup();
  await waitFor(() => expect(screen.getByRole("combobox", { name: "配置模板" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "配置模板" }));
  await user.click(await screen.findByRole("option", { name: template.name }));
  fail = true;
  await act(async () => {
    await client.invalidateQueries({ queryKey: templateKeys.library });
  });
  expect(await screen.findByText("模板刷新失败")).toBeVisible();
  expect(screen.getByRole("combobox", { name: "配置模板" })).toHaveTextContent(template.name);
  expect(screen.getByRole("combobox", { name: "配置模板" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("combobox", { name: "处理方式" }));
  await user.click(await screen.findByRole("option", { name: "独立 JSON 输出" }));
  await user.click(screen.getByRole("combobox", { name: "处理方式" }));
  await user.click(await screen.findByRole("option", { name: "导入目标站点" }));
  expect(screen.getByRole("combobox", { name: "配置模板" })).toHaveTextContent(template.name);
});
