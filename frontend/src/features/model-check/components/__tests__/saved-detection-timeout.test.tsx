import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { SavedDetectionTimeout } from "../saved-detection-timeout";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});
function setup(onSubmit = vi.fn()): void {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <SavedDetectionTimeout mode="animation" />
        <SavedDetectionTimeout mode="precheck" />
      </form>
    </QueryClientProvider>,
  );
}
it("编辑超时仅在点击旁边的保存后持久化，动画与前置设置相互独立", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === "PUT") {
        const value = JSON.parse(String(init.body));
        writes.push({ url, ...value });
        return Response.json({ ...value, version: 1 });
      }
      return Response.json({ timeout_seconds: 120, version: 0 });
    }),
  );
  setup();
  const user = userEvent.setup();
  const field = await screen.findByRole("spinbutton", { name: "动画超时（秒）" });
  expect(field).toHaveValue(120);
  expect(screen.getByRole("button", { name: "保存动画超时" })).toBeDisabled();
  await user.clear(field);
  await user.type(field, "75");
  expect(writes).toEqual([]);
  expect(screen.queryByText(/修改后保存生效/)).not.toBeInTheDocument();
  expect(field).not.toHaveAttribute("aria-describedby");
  await user.click(screen.getByRole("button", { name: "保存动画超时" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存动画超时" })).toBeDisabled());
  expect(screen.getByRole("spinbutton", { name: "动画超时（秒）" })).toHaveValue(75);
  expect(writes).toEqual([
    { url: "/api/model-checks/timeouts/animation", timeout_seconds: 75, version: 0 },
  ]);
  expect(screen.getByRole("spinbutton", { name: "前置检测超时（秒）" })).toHaveValue(120);
  expect(screen.getByRole("button", { name: "保存动画超时" })).toBeDisabled();
});
it("超时超出范围时阻止写入，回车只保存当前超时而不提交外层表单", async () => {
  const submit = vi.fn();
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === "PUT") {
        const value = JSON.parse(String(init.body));
        writes.push(value);
        return Response.json({ ...value, version: 1 });
      }
      return Response.json({ timeout_seconds: 120, version: 0 });
    }),
  );
  setup(submit);
  const user = userEvent.setup();
  const field = await screen.findByRole("spinbutton", { name: "动画超时（秒）" });
  await user.clear(field);
  await user.type(field, "121{Enter}");
  await waitFor(() => expect(field).toHaveAttribute("aria-invalid", "true"));
  expect(writes).toEqual([]);
  await user.clear(field);
  await user.type(field, "90{Enter}");
  await waitFor(() => expect(screen.getByRole("button", { name: "保存动画超时" })).toBeDisabled());
  expect(screen.getByRole("spinbutton", { name: "动画超时（秒）" })).toHaveValue(90);
  expect(submit).not.toHaveBeenCalled();
});
it("保存失败保留编辑草稿并提供重新读取", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) =>
      init?.method === "PUT"
        ? Response.json({ detail: "存储暂不可用" }, { status: 500 })
        : Response.json({ timeout_seconds: 60, version: 2 }),
    ),
  );
  setup();
  const user = userEvent.setup();
  const field = await screen.findByRole("spinbutton", { name: "动画超时（秒）" });
  await user.clear(field);
  await user.type(field, "90");
  await user.click(screen.getByRole("button", { name: "保存动画超时" }));
  expect(await screen.findByRole("button", { name: "重新读取动画超时" })).toBeEnabled();
  expect(field).toHaveValue(90);
});
