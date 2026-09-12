import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BrowserLogin } from "../browser-login";
const waiting = {
  id: "browser-1",
  task_id: "task-1",
  host: "login.example",
  status: "waiting",
  message: "等待登录",
  expires_at: "2030-01-01T00:00:00Z",
  image: "data:image/jpeg;base64,ZnJhbWU=",
  width: 1100,
  height: 760,
};
function setup(fetcher: typeof fetch): { client: QueryClient; unmount: () => void } {
  vi.stubGlobal("fetch", fetcher);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const result = render(
    <QueryClientProvider client={client}>
      <BrowserLogin host="login.example" />
    </QueryClientProvider>,
  );
  return { client, unmount: result.unmount };
}
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
describe("浏览器验证会话", () => {
  it("启动期间保持关闭入口且不能提交未就绪的凭据", async () => {
    let complete: ((response: Response) => void) | undefined;
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async (_url, init) => {
      if (init?.method === "POST")
        return new Promise<Response>((resolve) => {
          complete = resolve;
        });
      return Response.json({ cancelled: true });
    });
    const { client, unmount } = setup(fetcher);
    await userEvent.click(screen.getByRole("button", { name: "打开浏览器手动验证" }));
    expect(screen.getByRole("status", { name: "正在启动验证浏览器" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "登录完成，复核并保存" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "关闭验证" }));
    complete?.(Response.json(waiting));
    await waitFor(() =>
      expect(fetcher).toHaveBeenCalledWith(
        expect.stringContaining("/browser/browser-1"),
        expect.objectContaining({ method: "DELETE", credentials: "include" }),
      ),
    );
    unmount();
    client.clear();
  });
  it("关闭已就绪的浏览器会话后取消后端会话并移除画面缓存", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async (_url, init) =>
        Response.json(init?.method === "DELETE" ? { cancelled: true } : waiting),
      );
    const { client, unmount } = setup(fetcher);
    await userEvent.click(screen.getByRole("button", { name: "打开浏览器手动验证" }));
    await screen.findByRole("button", { name: "上游登录页面" });
    await userEvent.click(screen.getByRole("button", { name: "关闭验证" }));
    await waitFor(() =>
      expect(fetcher).toHaveBeenCalledWith(
        expect.stringContaining("/browser/browser-1"),
        expect.objectContaining({ method: "DELETE" }),
      ),
    );
    expect(client.getQueryData(["browser-login", "browser-1"])).toBeUndefined();
    unmount();
    client.clear();
  });
  it("明确点击完成后才请求后端复核", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async (url) =>
        Response.json(String(url).endsWith("/finish") ? { accepted: true } : waiting),
      );
    const { client, unmount } = setup(fetcher);
    await userEvent.click(screen.getByRole("button", { name: "打开浏览器手动验证" }));
    await screen.findByRole("button", { name: "上游登录页面" });
    expect(fetcher.mock.calls.some(([url]) => String(url).endsWith("/finish"))).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "登录完成，复核并保存" }));
    await waitFor(() =>
      expect(fetcher).toHaveBeenCalledWith(
        expect.stringContaining("/browser/browser-1/finish"),
        expect.objectContaining({ method: "POST" }),
      ),
    );
    unmount();
    client.clear();
  });
});
