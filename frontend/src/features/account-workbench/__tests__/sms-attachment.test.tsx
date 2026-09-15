import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkbenchOAuthSMSAttachment } from "../components/workbench-oauth-sms-attachment";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function mount(options: { read?: () => Promise<Response>; save?: () => Promise<Response> } = {}) {
  const requests: Array<{ method: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (_input, init) => {
      const method = init?.method ?? "GET";
      requests.push({
        method,
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      });
      if (method === "POST") return options.save?.() ?? Response.json({ accepted: true });
      return (
        options.read?.() ??
        Response.json({
          scope: "local-export",
          stage: "phone",
          revision: "page-version",
          configured: false,
          can_attach: true,
        })
      );
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchOAuthSMSAttachment id="current-oauth" disabled={false} />
    </QueryClientProvider>,
  );
  return { client, requests };
}
async function open(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  await user.click(screen.getByRole("button", { name: "配置短信接码" }));
  return screen.getByRole("dialog", { name: "配置当前授权的短信接码" });
}
async function fill(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  const dialog = await open(user);
  await user.click(await within(dialog).findByRole("combobox", { name: "短信验证码" }));
  await user.click(await screen.findByRole("option", { name: "LubanSMS" }));
  await user.type(within(dialog).getByLabelText("接码 API Key"), "isolated-sms-key");
  await user.type(within(dialog).getByLabelText("LubanSMS 供应商编号"), "isolated-service");
  return dialog;
}

describe("当前授权接码配置", () => {
  it("手机号步骤确认费用后提交当前版本，提交时清除凭据且不进入变更缓存", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const view = mount({
      save: () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    });
    const user = userEvent.setup();
    const dialog = await fill(user);
    await user.click(within(dialog).getByRole("button", { name: "确认用于当前手机号步骤" }));
    expect(view.requests.some((item) => item.method === "POST")).toBe(false);
    const confirm = within(dialog).getByRole("checkbox", {
      name: "我确认绑定接码手机号，并承担供应商费用",
    });
    expect(confirm).toHaveAttribute("aria-invalid", "true");
    await user.click(confirm);
    await user.click(within(dialog).getByRole("button", { name: "确认用于当前手机号步骤" }));
    await screen.findByRole("status", { name: "正在提交接码配置" });
    expect(screen.queryByLabelText("接码 API Key")).not.toBeInTheDocument();
    expect(view.requests.find((item) => item.method === "POST")?.body).toEqual({
      scope: "local-export",
      revision: "page-version",
      sms: {
        provider: "luban",
        api_key: "isolated-sms-key",
        service_id: "isolated-service",
        confirmed: true,
      },
    });
    expect(
      JSON.stringify(
        view.client
          .getMutationCache()
          .getAll()
          .map((item) => item.state.variables),
      ),
    ).not.toContain("isolated-sms-key");
    await user.click(within(dialog).getByRole("button", { name: "返回授权页面" }));
    await act(async () => resolve(Response.json({ accepted: true })));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(view.requests.filter((item) => item.method === "POST")).toHaveLength(1);
    expect(
      view.client.getQueryData(["account-workbench", "oauth-sms-attachment", "current-oauth"]),
    ).toBeUndefined();
  });
  it("已有配置或订单时不提供再次购号表单", async () => {
    const view = mount({
      read: async () => Response.json({ scope: "managed", configured: true, can_attach: false }),
    });
    await open(userEvent.setup());
    await screen.findByText("当前授权已有接码配置或订单，请先核对原订单");
    expect(screen.queryByRole("combobox", { name: "短信验证码" })).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "确认用于当前手机号步骤" }),
    ).not.toBeInTheDocument();
    expect(view.requests.some((item) => item.method === "POST")).toBe(false);
  });
  it("状态读取失败保留关闭和重试入口，重读到非手机号步骤后禁止配置", async () => {
    let reads = 0;
    mount({
      read: async () =>
        ++reads === 1
          ? Response.json({ detail: "页面不可读取" }, { status: 503 })
          : Response.json({
              scope: "managed",
              stage: "manual",
              configured: false,
              can_attach: false,
            }),
    });
    const user = userEvent.setup();
    const dialog = await open(user);
    expect(within(dialog).getByRole("button", { name: "返回授权页面" })).toBeEnabled();
    await user.click(await screen.findByRole("button", { name: "重新读取" }));
    await screen.findByText("当前授权不在可配置的手机号步骤");
    expect(
      screen.queryByRole("button", { name: "确认用于当前手机号步骤" }),
    ).not.toBeInTheDocument();
  });
  it("提交响应不明后只重读状态，不保留密钥或自动再发请求", async () => {
    let reads = 0;
    const view = mount({
      read: async () =>
        Response.json({
          scope: "managed",
          stage: "phone",
          revision: "page-version",
          configured: ++reads > 1,
          can_attach: reads === 1,
        }),
      save: async () => Response.json({ detail: "接码提交结果待核对" }, { status: 502 }),
    });
    const user = userEvent.setup();
    const dialog = await fill(user);
    await user.click(
      within(dialog).getByRole("checkbox", { name: "我确认绑定接码手机号，并承担供应商费用" }),
    );
    await user.click(within(dialog).getByRole("button", { name: "确认用于当前手机号步骤" }));
    await screen.findByText("当前授权已有接码配置或订单，请先核对原订单");
    expect(view.requests.filter((item) => item.method === "POST")).toHaveLength(1);
    expect(screen.queryByLabelText("接码 API Key")).not.toBeInTheDocument();
  });
});
