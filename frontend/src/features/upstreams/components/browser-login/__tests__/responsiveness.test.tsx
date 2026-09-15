import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import { BrowserLogin } from "../browser-login";

const waiting = {
  id: "browser-1",
  task_id: "task-1",
  host: "login.example",
  status: "waiting",
  message: "等待登录",
  expires_at: "2030-01-01T00:00:00Z",
  image: "data:image/jpeg;base64,b2xk",
  width: 1100,
  height: 760,
};
const clients: QueryClient[] = [];

async function openBrowser(fetcher: typeof fetch): Promise<QueryClient> {
  vi.stubGlobal("fetch", fetcher);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <BrowserLogin host="login.example" />
      <Toaster />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "打开浏览器手动验证" }));
  await screen.findByRole("button", { name: "上游登录页面" });
  await waitFor(() => expect(client.isFetching()).toBe(0));
  return client;
}

afterEach(() => {
  toast.dismiss();
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("远程浏览器操作反馈", () => {
  it("页面刷新失败后保留画面并恢复重试入口，不自动重放操作", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(async (url) =>
        String(url).endsWith("/input")
          ? Response.json({ detail: "验证页面暂时无法加载" }, { status: 503 })
          : Response.json(waiting),
      );
    await openBrowser(fetcher);
    fireEvent.click(screen.getByRole("button", { name: "刷新验证页面" }));
    await screen.findByText("验证页面暂时无法加载");
    await waitFor(() => expect(screen.getByRole("button", { name: "刷新验证页面" })).toBeEnabled());
    expect(screen.getByRole("img")).toHaveAttribute("src", waiting.image);
    expect(fetcher.mock.calls.filter(([url]) => String(url).endsWith("/input"))).toHaveLength(1);
  });

  it("上游验证失败显示错误码和下一步，同一错误随画面刷新不重复提示", async () => {
    const client = await openBrowser(async () =>
      Response.json({ ...waiting, challenge_code: "600010" }),
    );
    const message = /Cloudflare 验证失败（600010）.*上游管理员/;
    await screen.findByText(message);
    await act(async () => client.invalidateQueries({ queryKey: ["browser-login", waiting.id] }));
    expect(screen.getAllByText(message)).toHaveLength(1);
    expect(screen.getByRole("dialog")).not.toHaveTextContent("600010");
    expect(screen.getByRole("button", { name: "刷新验证页面" })).toBeEnabled();
  });

  it("关闭时丢弃尚未发送的文字，不把输入保存在请求缓存", async () => {
    const inputs: unknown[] = [];
    let release: ((response: Response) => void) | undefined;
    const client = await openBrowser(async (url, init) => {
      if (String(url).endsWith("/input")) {
        inputs.push(JSON.parse(String(init?.body)) as unknown);
        return new Promise<Response>((resolve) => {
          release = resolve;
        });
      }
      return Response.json(waiting);
    });
    const surface = screen.getByRole("button", { name: "上游登录页面" });
    fireEvent.keyDown(surface, { key: "a" });
    await waitFor(() => expect(inputs).toHaveLength(1));
    fireEvent.keyDown(surface, { key: "b" });
    fireEvent.click(screen.getByRole("button", { name: "关闭验证" }));
    await act(async () => release?.(Response.json({ accepted: true })));
    expect(inputs).toEqual([{ kind: "text", text: "a" }]);
    expect(client.getQueryData(["browser-login", waiting.id])).toBeUndefined();
    expect(
      client
        .getMutationCache()
        .getAll()
        .some((entry) => JSON.stringify(entry.state.variables)?.includes('"text"')),
    ).toBe(false);
  });

  it("等待验证时每秒最多读取四帧，后台读取未完成时不会叠加请求", async () => {
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async () => Response.json(waiting));
    const client = await openBrowser(fetcher);
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    await act(async () => client.invalidateQueries({ queryKey: ["browser-login", waiting.id] }));
    fetcher.mockClear();
    await act(async () => vi.advanceTimersByTime(250));
    expect(fetcher).toHaveBeenCalledTimes(1);
    let release: ((response: Response) => void) | undefined;
    fetcher.mockImplementationOnce(
      () =>
        new Promise<Response>((resolve) => {
          release = resolve;
        }),
    );
    await act(async () => vi.advanceTimersByTime(250));
    await act(async () => vi.advanceTimersByTime(1000));
    expect(fetcher).toHaveBeenCalledTimes(2);
    await act(async () => release?.(Response.json(waiting)));
  });

  it("输入完成后立即读取新画面，不等待下一轮定时刷新", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    let edited = false;
    await openBrowser(async (url) => {
      if (String(url).endsWith("/input")) {
        edited = true;
        return Response.json({ accepted: true });
      }
      return Response.json({
        ...waiting,
        image: edited ? "data:image/jpeg;base64,bmV3" : waiting.image,
      });
    });
    fireEvent.keyDown(screen.getByRole("button", { name: "上游登录页面" }), { key: "a" });
    await waitFor(() =>
      expect(screen.getByRole("img")).toHaveAttribute("src", "data:image/jpeg;base64,bmV3"),
    );
  });

  it("网络尚未返回时合并连续文字，保留输入框切换顺序", async () => {
    const inputs: unknown[] = [];
    let release: ((response: Response) => void) | undefined;
    await openBrowser(async (url, init) => {
      if (!String(url).endsWith("/input")) return Response.json(waiting);
      inputs.push(JSON.parse(String(init?.body)) as unknown);
      if (inputs.length === 1)
        return new Promise<Response>((resolve) => {
          release = resolve;
        });
      return Response.json({ accepted: true });
    });
    const surface = screen.getByRole("button", { name: "上游登录页面" });
    fireEvent.keyDown(surface, { key: "a" });
    await waitFor(() => expect(inputs).toHaveLength(1));
    fireEvent.keyDown(surface, { key: "b" });
    fireEvent.keyDown(surface, { key: "c" });
    fireEvent.click(screen.getByRole("button", { name: "下一个输入框" }));
    fireEvent.keyDown(surface, { key: "d" });
    fireEvent.keyDown(surface, { key: "e" });
    await act(async () => release?.(Response.json({ accepted: true })));
    await waitFor(() =>
      expect(inputs).toEqual([
        { kind: "text", text: "a" },
        { kind: "text", text: "bc" },
        { kind: "key", key: "Tab" },
        { kind: "text", text: "de" },
      ]),
    );
  });

  it("验证失败时可刷新登录页，刷新完成前禁用输入与复核", async () => {
    let release: ((response: Response) => void) | undefined;
    const fetcher = vi.fn<typeof fetch>().mockImplementation(async (url) => {
      if (String(url).endsWith("/input"))
        return new Promise<Response>((resolve) => {
          release = resolve;
        });
      return Response.json(waiting);
    });
    await openBrowser(fetcher);
    fireEvent.click(screen.getByRole("button", { name: "刷新验证页面" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "上游登录页面" })).toHaveAttribute(
        "aria-disabled",
        "true",
      ),
    );
    expect(screen.getByRole("button", { name: "登录完成，复核并保存" })).toBeDisabled();
    expect(fetcher).toHaveBeenCalledWith(
      expect.stringContaining("/browser/browser-1/input"),
      expect.objectContaining({ method: "POST", body: JSON.stringify({ kind: "reload" }) }),
    );
    await act(async () => release?.(Response.json({ accepted: true })));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "上游登录页面" })).toHaveAttribute(
        "aria-disabled",
        "false",
      ),
    );
  });
});
