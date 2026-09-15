import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WorkbenchOAuth } from "../components/workbench-oauth";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
const price = "0.000000000000000000001";
const options = [{ country: "15", title: "测试国家", iso: "XX", prefix: "1202", price, count: 3 }];
function mount(
  optionsResponse?: () => Promise<Response>,
): Array<{ path: string; method: string; body: unknown }> {
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = String(input);
      const method = init?.method ?? "GET";
      requests.push({
        path,
        method,
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      });
      if (path.endsWith("/sms/options")) return optionsResponse?.() ?? Response.json(options);
      if (method === "DELETE") return Response.json({ cancelled: true });
      return Response.json({
        id: "sms-session",
        task_id: "sms-task",
        host: "auth.openai.com",
        status: "waiting",
        message: "等待完成授权",
        expires_at: new Date(Date.now() + 900000).toISOString(),
        width: 1100,
        height: 760,
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchOAuth />
    </QueryClientProvider>,
  );
  return requests;
}
async function openSMS(
  user: ReturnType<typeof userEvent.setup>,
  provider: string,
): Promise<HTMLElement> {
  await user.click(screen.getByRole("checkbox", { name: "自动填写登录信息" }));
  await user.click(screen.getByRole("button", { name: "开始授权登录" }));
  const dialog = screen.getByRole("dialog", { name: "自动填写登录信息" });
  await user.type(
    within(dialog).getByRole("textbox", { name: "登录邮箱" }),
    "operator@example.test",
  );
  await user.click(within(dialog).getByRole("combobox", { name: "短信验证码" }));
  await user.click(await screen.findByRole("option", { name: provider }));
  return dialog;
}

describe("授权短信接码", () => {
  it("自定义接码需确认手机号绑定后才把接码列表发送给后端", async () => {
    const requests = mount();
    const user = userEvent.setup();
    const dialog = await openSMS(user, "自定义接码");
    const entries = "+12025550142----https://sms.example.test/code";
    await user.type(within(dialog).getByRole("textbox", { name: "自定义接码列表" }), entries);
    await user.click(within(dialog).getByRole("button", { name: "开始授权登录" }));
    expect(requests.some((item) => item.path.endsWith("/oauth"))).toBe(false);
    const confirmed = within(dialog).getByRole("checkbox", {
      name: "我确认绑定接码手机号，并承担供应商费用",
    });
    expect(confirmed).toHaveAttribute("aria-invalid", "true");
    await user.click(confirmed);
    await user.click(within(dialog).getByRole("button", { name: "开始授权登录" }));
    await waitFor(() =>
      expect(requests.find((item) => item.path.endsWith("/oauth"))?.body).toEqual({
        login: {
          email: "operator@example.test",
          sms: { provider: "custom", custom_entries: entries, confirmed: true },
        },
      }),
    );
  });

  it("SMSBower 不预填国家或价格，读取后按选项原值提交价格上限", async () => {
    const requests = mount();
    const user = userEvent.setup();
    const dialog = await openSMS(user, "SMSBower");
    expect(within(dialog).getByRole("combobox", { name: "接码国家" })).toBeDisabled();
    await user.type(within(dialog).getByLabelText("接码 API Key"), "fixture-key");
    await user.click(within(dialog).getByRole("button", { name: "读取国家价格" }));
    await waitFor(() =>
      expect(within(dialog).getByRole("combobox", { name: "接码国家" })).toBeEnabled(),
    );
    expect(within(dialog).getByRole("combobox", { name: "接码国家" })).toHaveTextContent(
      "请选择国家",
    );
    await user.click(within(dialog).getByRole("combobox", { name: "接码国家" }));
    await user.click(await screen.findByRole("option", { name: /测试国家/ }));
    await user.click(
      within(dialog).getByRole("checkbox", { name: "我确认绑定接码手机号，并承担供应商费用" }),
    );
    await user.click(within(dialog).getByRole("button", { name: "开始授权登录" }));
    await waitFor(() =>
      expect(requests.find((item) => item.path.endsWith("/oauth"))?.body).toEqual({
        login: {
          email: "operator@example.test",
          sms: {
            provider: "smsbower",
            api_key: "fixture-key",
            country: "15",
            max_price: price,
            confirmed: true,
          },
        },
      }),
    );
    expect(requests.find((item) => item.path.endsWith("/sms/options"))?.body).toEqual({
      provider: "smsbower",
      api_key: "fixture-key",
    });
  });

  it("变更 API Key 会清除所选价格和费用确认", async () => {
    mount();
    const user = userEvent.setup();
    const dialog = await openSMS(user, "SMSBower");
    const key = within(dialog).getByLabelText("接码 API Key");
    await user.type(key, "fixture-key");
    await user.click(within(dialog).getByRole("button", { name: "读取国家价格" }));
    await waitFor(() =>
      expect(within(dialog).getByRole("combobox", { name: "接码国家" })).toBeEnabled(),
    );
    await user.click(within(dialog).getByRole("combobox", { name: "接码国家" }));
    await user.click(await screen.findByRole("option", { name: /测试国家/ }));
    const confirmed = within(dialog).getByRole("checkbox", {
      name: "我确认绑定接码手机号，并承担供应商费用",
    });
    await user.click(confirmed);
    await user.type(key, "-changed");
    expect(confirmed).not.toBeChecked();
    expect(within(dialog).getByRole("combobox", { name: "接码国家" })).toBeDisabled();
    expect(within(dialog).getByRole("combobox", { name: "接码国家" })).toHaveTextContent(
      "请选择国家",
    );
  });

  it("切换供应商后忽略尚未返回的旧国家价格并清空 Key", async () => {
    let resolveOptions: (response: Response) => void = () => undefined;
    const response = new Promise<Response>((resolve) => {
      resolveOptions = resolve;
    });
    mount(() => response);
    const user = userEvent.setup();
    const dialog = await openSMS(user, "SMSBower");
    await user.type(within(dialog).getByLabelText("接码 API Key"), "fixture-key");
    await user.click(within(dialog).getByRole("button", { name: "读取国家价格" }));
    expect(
      within(dialog).getByRole("status", { name: "正在读取接码国家价格" }),
    ).toBeInTheDocument();
    await user.click(within(dialog).getByRole("combobox", { name: "短信验证码" }));
    await user.click(await screen.findByRole("option", { name: "LubanSMS" }));
    resolveOptions(Response.json(options));
    expect(within(dialog).getByLabelText("接码 API Key")).toHaveValue("");
    await user.click(within(dialog).getByRole("combobox", { name: "短信验证码" }));
    await user.click(await screen.findByRole("option", { name: "SMSBower" }));
    await waitFor(() =>
      expect(within(dialog).getByRole("button", { name: "读取国家价格" })).toBeEnabled(),
    );
    expect(within(dialog).getByRole("combobox", { name: "接码国家" })).toBeDisabled();
  });

  it("国家价格请求失败后保留重试与关闭入口", async () => {
    mount(async () =>
      Response.json({ detail: "供应商价格暂不可用", code: "sms_options_failed" }, { status: 503 }),
    );
    const user = userEvent.setup();
    const dialog = await openSMS(user, "SMSBower");
    await user.type(within(dialog).getByLabelText("接码 API Key"), "fixture-key");
    await user.click(within(dialog).getByRole("button", { name: "读取国家价格" }));
    await waitFor(() =>
      expect(within(dialog).getByRole("button", { name: "读取国家价格" })).toBeEnabled(),
    );
    expect(within(dialog).getByRole("button", { name: "取消" })).toBeEnabled();
    expect(within(dialog).getByRole("combobox", { name: "接码国家" })).toBeDisabled();
  });
});
