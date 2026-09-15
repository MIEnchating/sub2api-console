import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkbenchSecurityBatch } from "../components/workbench-security-batch";
import type { WorkbenchSecurityBatchPreview } from "@/api";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
const preview = (): WorkbenchSecurityBatchPreview => ({
  id: "scope-1",
  target: "https://sub2api.example",
  operation: "totp",
  expires_at: new Date(Date.now() + 600000).toISOString(),
  items: [
    {
      index: 0,
      account_id: "42",
      email: "owner@example.com",
      user_id: "user-42",
      status: "queued",
      message: "等待处理",
    },
  ],
  errors: [],
});
function mount(previewResponse?: () => Promise<Response>) {
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = String(input),
        method = init?.method ?? "GET";
      requests.push({
        path,
        method,
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      });
      if (path === "/api/accounts")
        return Response.json([
          { id: "42", name: "团队账号", platform: "openai", account_type: "oauth" },
        ]);
      if (method === "DELETE") return Response.json({ cancelled: true, deleted: true });
      if (path.endsWith("/preview")) return previewResponse?.() ?? Response.json(preview());
      return Response.json({
        id: "batch-1",
        task_id: "batch-1",
        operation: "totp",
        status: "running",
        message: "正在处理所选账号",
        completed: 0,
        succeeded: 0,
        expires_at: new Date(Date.now() + 7200000).toISOString(),
        items: preview().items,
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const result = render(
    <QueryClientProvider client={client}>
      <WorkbenchSecurityBatch />
    </QueryClientProvider>,
  );
  return { requests, client, unmount: result.unmount };
}
async function selectAndPreview(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(await screen.findByRole("checkbox", { name: "团队账号（ID 42）" }));
  await user.click(screen.getByRole("button", { name: "预览批量安全操作" }));
}
describe("批量账号安全设置", () => {
  it("确认范围时只发送预览 ID 并通过二次确认结束批次", async () => {
    const view = mount(),
      user = userEvent.setup();
    await selectAndPreview(user);
    await screen.findByRole("region", { name: "批量安全操作预览" });
    expect(view.requests.filter((v) => v.method === "POST")).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: "确认并执行批量安全操作" }));
    await screen.findByRole("region", { name: "批量安全操作结果" });
    expect(
      view.requests.find((v) => v.path.endsWith("/security-batches") && v.method === "POST")?.body,
    ).toEqual({ preview_id: "scope-1", confirmed: true });
    await user.click(screen.getByRole("button", { name: "结束批量安全设置" }));
    expect(view.requests.some((v) => v.method === "DELETE" && v.path.endsWith("batch-1"))).toBe(
      false,
    );
    await user.click(screen.getByRole("button", { name: "确认结束本批" }));
    await waitFor(() =>
      expect(view.requests.some((v) => v.method === "DELETE" && v.path.endsWith("batch-1"))).toBe(
        true,
      ),
    );
  });
  it("关闭未完成的预览会删除迟到的范围并保持表单可用", async () => {
    let resolve: (value: Response) => void = () => undefined;
    const response = new Promise<Response>((done) => {
      resolve = done;
    });
    const view = mount(() => response),
      user = userEvent.setup();
    await selectAndPreview(user);
    await user.click(screen.getByRole("button", { name: "结束批量安全设置" }));
    resolve(Response.json(preview()));
    await waitFor(() =>
      expect(view.requests.some((v) => v.method === "DELETE" && v.path.endsWith("scope-1"))).toBe(
        true,
      ),
    );
    expect(screen.queryByRole("region", { name: "批量安全操作预览" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "预览批量安全操作" })).toBeEnabled();
  });
  it("预览包含无效身份时禁用执行并允许返回修改", async () => {
    mount(async () =>
      Response.json({
        ...preview(),
        id: "",
        items: [],
        errors: [{ index: 0, message: "账号缺少稳定身份" }],
      }),
    );
    const user = userEvent.setup();
    await selectAndPreview(user);
    expect(await screen.findByText("第 1 项：账号缺少稳定身份")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "确认并执行批量安全操作" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "返回修改范围" }));
    expect(screen.getByRole("button", { name: "预览批量安全操作" })).toBeEnabled();
  });
});
