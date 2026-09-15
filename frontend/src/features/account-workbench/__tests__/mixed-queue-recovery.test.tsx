import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { WorkbenchRunView, WorkbenchScope } from "@/api";
import { WorkbenchMixed } from "../components/workbench-mixed";
import { workbenchKeys } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
const queue = {
  id: "mixed-queue",
  kind: "mixed",
  task_id: "original-mixed",
  status: "interrupted",
  revision: 7,
  expires_at: new Date(Date.now() + 600000).toISOString(),
  can_resume: true,
};
const restored: WorkbenchRunView = {
  id: "restored-mixed",
  task_id: "restored-task",
  status: "ready",
  message: "已恢复成功账号",
  expires_at: queue.expires_at,
  export_only: true,
  available: 1,
  recovery_enabled: true,
  recovery_id: queue.id,
  errors: [],
  items: [
    {
      index: 0,
      kind: "codex_json",
      name: "已保存账号",
      has_password: false,
      has_proxy: false,
      has_totp: false,
      status: "succeeded",
      message: "凭据已保存",
    },
  ],
};
function mount(scope?: WorkbenchScope): {
  fetcher: ReturnType<typeof vi.fn<typeof fetch>>;
  unmount: () => void;
} {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const fetcher = vi.fn<typeof fetch>(async (url, options) => {
    if (String(url).includes("queue-recoveries") && options?.method !== "POST")
      return Response.json([queue, { ...queue, id: "other-kind", kind: "oauth-batch" }]);
    if (String(url).includes("templates")) return Response.json([]);
    if (options?.method === "DELETE") return Response.json({ cancelled: true });
    return Response.json({ ...restored, scope });
  });
  vi.stubGlobal("fetch", fetcher);
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <WorkbenchMixed scope={scope} />
    </QueryClientProvider>,
  );
  return { fetcher, unmount: view.unmount };
}
async function resume(): Promise<void> {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "恢复已保存批次" }));
  await user.click(await screen.findByRole("button", { name: "恢复本批" }));
  await user.click(screen.getByRole("button", { name: "确认继续本批" }));
  await screen.findByRole("region", { name: "混合运行进度" });
}

it("混合恢复筛选父批次并在确认后提交稳定ID、版本和本地范围", async () => {
  const view = mount("local-export");
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "恢复已保存批次" }));
  expect(await screen.findAllByRole("button", { name: "恢复本批" })).toHaveLength(1);
  await user.click(screen.getByRole("button", { name: "恢复本批" }));
  expect(view.fetcher.mock.calls.some(([, options]) => options?.method === "POST")).toBe(false);
  await user.click(screen.getByRole("button", { name: "确认继续本批" }));
  expect(await screen.findByRole("region", { name: "混合运行进度" })).toHaveTextContent(
    "restored-task",
  );
  const request = view.fetcher.mock.calls.find(
    ([url, options]) =>
      String(url).endsWith("/queue-recoveries/mixed-queue/mixed") && options?.method === "POST",
  );
  expect(JSON.parse(String(request?.[1]?.body))).toEqual({
    revision: 7,
    confirmed: true,
    scope: "local-export",
  });
  expect(screen.getByRole("button", { name: "恢复已保存批次" })).toBeDisabled();
});

it("启用恢复的批次离开页面仅清理查询且不删除服务器保存结果", async () => {
  const view = mount();
  await resume();
  await act(async () => {
    window.dispatchEvent(new Event("pagehide"));
  });
  view.unmount();
  expect(view.fetcher.mock.calls.some(([, options]) => options?.method === "DELETE")).toBe(false);
  expect(client.getQueryData(workbenchKeys.run(restored.id))).toBeUndefined();
});

it("主动结束已恢复批次仍需确认并清除服务器结果", async () => {
  const view = mount();
  await resume();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "结束混合运行" }));
  expect(view.fetcher.mock.calls.some(([, options]) => options?.method === "DELETE")).toBe(false);
  await user.click(screen.getByRole("button", { name: "结束并清除混合结果" }));
  await waitFor(() =>
    expect(screen.queryByRole("region", { name: "混合运行进度" })).not.toBeInTheDocument(),
  );
  expect(view.fetcher).toHaveBeenCalledWith(
    "/api/account-workbench/runs/restored-mixed",
    expect.objectContaining({ method: "DELETE" }),
  );
});
