import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AnimationQualitySettingsDialog } from "../animation-quality-settings";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});
function setup(): void {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <AnimationQualitySettingsDialog />
    </QueryClientProvider>,
  );
}
it("启用自动标注后要求账号和模型，保存稳定账号及配置版本", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === "PUT") {
        const body = JSON.parse(String(init.body));
        writes.push(body);
        return Response.json({ ...body, version: 4 });
      }
      if (url.endsWith("/accounts")) return Response.json([{ id: "41", name: "评审账号甲" }]);
      if (url.endsWith("/models")) return Response.json({ models: ["review-model"] });
      return Response.json({
        enabled: false,
        account_id: "",
        model: "",
        timeout_seconds: 60,
        version: 3,
      });
    }),
  );
  setup();
  await user.click(screen.getByRole("button", { name: "自动标注设置" }));
  const account = await screen.findByLabelText("评审账号");
  expect(account).toBeDisabled();
  await user.click(screen.getByRole("checkbox", { name: "生成后自动标注质量" }));
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  expect(await screen.findByText("请选择评审账号")).toBeVisible();
  expect(account).toHaveAttribute("aria-invalid", "true");
  expect(writes).toEqual([]);
  await user.click(account);
  await user.click(screen.getByRole("option", { name: "评审账号甲 · 41" }));
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  const model = screen.getByRole("combobox", { name: "评审模型" });
  await waitFor(() => expect(model).toBeEnabled());
  await user.click(model);
  await user.click(screen.getByRole("option", { name: "review-model" }));
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(writes).toEqual([
    { enabled: true, account_id: "41", model: "review-model", timeout_seconds: 60, version: 3 },
  ]);
});
it("设置读取失败时保留取消和重试，数据就绪前不可保存", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ detail: "失败" }, { status: 500 })),
  );
  setup();
  await user.click(screen.getByRole("button", { name: "自动标注设置" }));
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeVisible();
  expect(screen.queryByRole("button", { name: "保存设置" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("单独保存评审超时保留账号模型草稿，随后保存设置使用新版本", async () => {
  const writes: { url: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === "PUT") {
        const body = JSON.parse(String(init.body));
        writes.push({ url, body });
        return Response.json({ ...body, version: body.version + 1 });
      }
      if (url.endsWith("/accounts")) return Response.json([{ id: "41", name: "评审账号甲" }]);
      if (url.endsWith("/models")) return Response.json({ models: ["draft-model"] });
      return Response.json({
        enabled: true,
        account_id: "41",
        model: "original-model",
        timeout_seconds: 120,
        version: 3,
      });
    }),
  );
  setup();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "自动标注设置" }));
  const model = await screen.findByRole("combobox", { name: "评审模型" });
  expect(model).toHaveTextContent("original-model");
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(model).toBeEnabled());
  await user.click(model);
  await user.click(screen.getByRole("option", { name: "draft-model" }));
  const timeout = await screen.findByRole("spinbutton", { name: "评审超时（秒）" });
  await user.clear(timeout);
  await user.type(timeout, "90");
  await user.click(screen.getByRole("button", { name: "保存评审超时" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存评审超时" })).toBeDisabled());
  expect(model).toHaveTextContent("draft-model");
  expect(writes).toEqual([
    { url: "/api/model-checks/timeouts/review", body: { timeout_seconds: 90, version: 3 } },
  ]);
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(writes[1]).toEqual({
    url: "/api/model-checks/animation-quality/settings",
    body: {
      enabled: true,
      account_id: "41",
      model: "draft-model",
      timeout_seconds: 90,
      version: 4,
    },
  });
});
it("长账号列表提供搜索和受限高度，键盘可以选择稳定账号", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      if (url.endsWith("/accounts"))
        return Response.json(
          Array.from({ length: 30 }, (_, index) => ({
            id: String(index),
            name: `评审账号 ${index}`,
          })),
        );
      return Response.json({
        enabled: true,
        account_id: "",
        model: "review",
        timeout_seconds: 120,
        version: 0,
      });
    }),
  );
  vi.stubGlobal("PointerEvent", MouseEvent);
  setup();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "自动标注设置" }));
  const account = await screen.findByRole("combobox", { name: "评审账号" });
  await user.click(account);
  const search = await screen.findByRole("textbox", { name: "搜索选项" });
  await user.type(search, "评审账号 29");
  expect(screen.getByRole("option", { name: "评审账号 29 · 29" })).toBeVisible();
  expect(screen.queryByRole("option", { name: "评审账号 1 · 1" })).not.toBeInTheDocument();
  await user.keyboard("{ArrowDown}{Enter}");
  expect(account).toHaveTextContent("评审账号 29");
  expect(account).toHaveAttribute("aria-expanded", "false");
});
