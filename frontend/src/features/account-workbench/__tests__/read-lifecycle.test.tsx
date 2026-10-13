import type { ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountList } from "../components/account-list";
import { ImportPanel } from "../components/import-panel";
import { MaintenancePanel } from "../components/maintenance-panel";
import { RunList } from "../components/run-list";
import { TemplateList } from "../components/template-list";
import { TemplateEditor } from "../components/template-editor";
import { importModelKey, maintenanceKey, runKeys, templateKeys, workbenchKeys } from "../constants";
import { template } from "./template-fixture";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function createClient(): QueryClient {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(importModelKey, { model: "gpt-6-astra", revision: 0 });
  clients.push(client);
  return client;
}

for (const scenario of [
  { label: "账号列表", component: AccountList, key: workbenchKeys.accounts, data: [] },
  {
    label: "模板列表",
    component: TemplateList,
    key: templateKeys.library,
    data: { revision: 1, preferred_id: "", items: [] },
  },
  { label: "处理记录", component: RunList, key: runKeys.list, data: [] },
  { label: "维护设置", component: MaintenancePanel, key: maintenanceKey, data: {} },
]) {
  it(`${scenario.label}离页取消在途读取，迟到响应不写入查询缓存`, async () => {
    let signal: AbortSignal | null | undefined;
    let finish!: (response: Response) => void;
    const fetcher = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    });
    vi.stubGlobal("fetch", fetcher);
    const client = createClient();
    const Component = scenario.component;
    const view = render(
      <QueryClientProvider client={client}>
        <Component />
      </QueryClientProvider>,
    );
    await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());
    view.unmount();
    await act(async () => finish(Response.json(scenario.data)));
    expect(signal?.aborted).toBe(true);
    expect(client.getQueryData(scenario.key)).toBeUndefined();
    expect(fetcher).toHaveBeenCalledOnce();
  });
}

it("关闭模板同步弹窗取消来源读取，迟到配置不留在缓存且不保存模板", async () => {
  let signal: AbortSignal | null | undefined;
  let finish!: (response: Response) => void;
  const fetcher = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  });
  vi.stubGlobal("fetch", fetcher);
  const client = createClient();
  client.setQueryData(templateKeys.library, {
    revision: 1,
    preferred_id: template.id,
    items: [template],
  });
  client.setQueryData(workbenchKeys.accounts, [template.summary]);
  render(
    <QueryClientProvider client={client}>
      <TemplateList />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "重新同步" }));
  await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());
  await user.click(screen.getByRole("button", { name: "取消" }));
  await act(async () => finish(Response.json(template)));
  expect(signal?.aborted).toBe(true);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(
    client.getQueryData(["account-workbench", "template-source", template.source_id]),
  ).toBeUndefined();
  expect(fetcher).toHaveBeenCalledOnce();
});

for (const scenario of ["templates", "accounts"] as const) {
  it(`关闭一个${scenario === "templates" ? "模板" : "账号"}读取入口时，共享观察者继续接收原请求`, async () => {
    let signal: AbortSignal | null | undefined;
    let finish!: (response: Response) => void;
    const fetcher = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    });
    vi.stubGlobal("fetch", fetcher);
    const client = createClient();
    const first: ReactElement =
      scenario === "templates" ? (
        <ImportPanel onStarted={() => undefined} />
      ) : (
        <TemplateEditor revision={0} onClose={() => undefined} />
      );
    const view = render(<QueryClientProvider client={client}>{first}</QueryClientProvider>);
    await waitFor(() => expect(fetcher).toHaveBeenCalledOnce());
    render(
      <QueryClientProvider client={client}>
        {scenario === "templates" ? <TemplateList /> : <AccountList />}
      </QueryClientProvider>,
    );
    view.unmount();
    expect(signal).toBeInstanceOf(AbortSignal);
    expect(signal?.aborted).toBe(false);
    const data =
      scenario === "templates"
        ? { revision: 1, preferred_id: template.id, items: [template] }
        : [template.summary];
    await act(async () => finish(Response.json(data)));
    if (scenario === "templates")
      expect(await screen.findByRole("article", { name: template.name })).toBeVisible();
    else expect(await screen.findByRole("button", { name: "查看 来源账号 配置" })).toBeVisible();
    expect(fetcher).toHaveBeenCalledOnce();
  });
}
