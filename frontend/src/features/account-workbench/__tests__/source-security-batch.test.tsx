import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkbenchSourceSecurityBatch } from "../components/workbench-source-security-batch";
import { WorkbenchSecurityBrowser } from "../components/workbench-security-browser";
import type { WorkbenchSecurityBatchSource } from "@/api";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function mount(child = false, options: { checkpoint?: boolean; running?: boolean } = {}) {
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  const source: WorkbenchSecurityBatchSource = options.checkpoint
    ? { checkpoint_id: "paused-source", revision: 2, checkpoint_revision: 1 }
    : { oauth_batch_id: "original-batch", index: 7 };
  const expires = new Date(Date.now() + 600000).toISOString();
  let confirmed = false;
  const batch = {
    id: "source-security-batch",
    task_id: "source-security-batch",
    scope: "local-export",
    status: options.running ? "running" : "succeeded",
    message: "安全操作结束",
    completed: 1,
    succeeded: 1,
    expires_at: expires,
    operation: "totp",
    items: [
      {
        index: 0,
        source,
        email: "local@example.test",
        user_id: "local-user",
        status: "succeeded",
        message: "设置完成",
        artifact_id: "private-security-result",
        security_id: "source-child",
      },
    ],
  };
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
      if (method === "DELETE") return Response.json({ cancelled: true });
      if (path.endsWith("/security/source-child/oauth"))
        return Response.json({ id: "new-oauth", status: "waiting", scope: "local-export" });
      if (path.endsWith("/security-batches/preview"))
        return Response.json({
          id: "source-preview",
          scope: "local-export",
          target: "",
          expires_at: expires,
          operation: "totp",
          items: batch.items,
          errors: [],
        });
      if (
        path.endsWith("/security-batches") ||
        path.endsWith("/security-batches/source-security-batch")
      )
        return Response.json(batch);
      if (path.endsWith("/confirm-identity")) {
        confirmed = true;
        return Response.json({ accepted: true });
      }
      if (path.endsWith("/security/source-child"))
        return Response.json({
          id: "source-child",
          task_id: "source-child",
          source_checkpoint_id: "paused-source",
          scope: "local-export",
          status: confirmed ? "waiting" : "awaiting_confirmation",
          email: "official@example.test",
          user_id: "official-user",
          message: "等待确认",
          identity_confirmed: confirmed,
          expires_at: expires,
          width: 1100,
          height: 760,
        });
      throw new Error(`未预期请求 ${method} ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const onOAuth = vi.fn();
  const view = render(
    <QueryClientProvider client={client}>
      {child ? (
        <WorkbenchSecurityBrowser id="source-child" />
      ) : (
        <WorkbenchSourceSecurityBatch
          scope="local-export"
          sources={[
            { source, label: "第 8 项：local@example.test" },
            { source: { oauth_id: "other-oauth" }, label: "其他授权账号" },
          ]}
          onClose={() => undefined}
          onOAuth={onOAuth}
        />
      )}
    </QueryClientProvider>,
  );
  return { ...view, requests, client, onOAuth };
}
describe("授权来源批量安全设置", () => {
  it("选择批次成功来源保留原序号并预览确认，不使用托管账号ID", async () => {
    const view = mount();
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "第 8 项：local@example.test" }));
    await user.click(screen.getByRole("button", { name: "预览批量安全操作" }));
    const preview = await screen.findByRole("region", { name: "批量安全操作预览" });
    expect(preview).toHaveTextContent("范围：本地授权来源");
    expect(preview).toHaveTextContent("local-user");
    expect(view.requests[0].body).toEqual({
      sources: [{ oauth_batch_id: "original-batch", index: 7 }],
      scope: "local-export",
      operation: "totp",
    });
    expect(view.requests.some((item) => item.path === "/api/accounts")).toBe(false);
    await user.click(within(preview).getByRole("button", { name: "确认并执行批量安全操作" }));
    await screen.findByRole("region", { name: "批量安全操作结果" });
    expect(view.requests.find((item) => item.path.endsWith("/security-batches"))?.body).toEqual({
      preview_id: "source-preview",
      confirmed: true,
    });
    expect(screen.getByRole("region", { name: "批量安全操作结果" })).not.toHaveTextContent(
      "ID undefined",
    );
    view.unmount();
    await waitFor(() => expect(view.requests.some((item) => item.method === "DELETE")).toBe(true));
    expect(
      view.requests.some((item) => item.path.includes("oauth-batches") && item.method === "DELETE"),
    ).toBe(false);
  });
  it("未选择任何来源时阻止预览", async () => {
    const view = mount();
    await userEvent.setup().click(screen.getByRole("button", { name: "预览批量安全操作" }));
    expect(await screen.findByText("请选择一种账号来源及至少一个账号")).toBeInTheDocument();
    expect(view.requests).toEqual([]);
  });
  it("批量中的检查点账号需单独确认官方身份后才允许继续", async () => {
    const view = mount(true);
    const identity = await screen.findByRole("region", { name: "确认当前官方账号身份" });
    expect(identity).toHaveTextContent("official-user");
    expect(screen.getByRole("button", { name: "验证完成，继续当前账号" })).toBeDisabled();
    await userEvent
      .setup()
      .click(within(identity).getByRole("button", { name: "确认当前官方账号" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "验证完成，继续当前账号" })).toBeEnabled(),
    );
    expect(view.requests.find((item) => item.path.endsWith("/confirm-identity"))?.body).toEqual({
      email: "official@example.test",
      user_id: "official-user",
      confirmed: true,
    });
  });
  it("已结束批次里的成功检查点结果可显式交给新授权页面", async () => {
    const view = mount(false, { checkpoint: true });
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "第 8 项：local@example.test" }));
    await user.click(screen.getByRole("button", { name: "预览批量安全操作" }));
    await user.click(await screen.findByRole("button", { name: "确认并执行批量安全操作" }));
    await user.click(await screen.findByRole("button", { name: "确认发起新的 OAuth 授权" }));
    await waitFor(() =>
      expect(view.onOAuth).toHaveBeenCalledWith(expect.objectContaining({ id: "new-oauth" })),
    );
    expect(
      view.requests.find((item) => item.path.endsWith("/security/source-child/oauth"))?.body,
    ).toEqual({ confirmed: true });
  });
  it("批次仍在处理其他项目时不提供新授权操作", async () => {
    const view = mount(false, { checkpoint: true, running: true });
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: "第 8 项：local@example.test" }));
    await user.click(screen.getByRole("button", { name: "预览批量安全操作" }));
    await user.click(await screen.findByRole("button", { name: "确认并执行批量安全操作" }));
    await screen.findByRole("region", { name: "批量安全操作结果" });
    expect(
      screen.queryByRole("button", { name: "确认发起新的 OAuth 授权" }),
    ).not.toBeInTheDocument();
    expect(view.requests.some((item) => item.path.endsWith("/security/source-child/oauth"))).toBe(
      false,
    );
  });
});
