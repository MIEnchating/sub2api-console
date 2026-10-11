import { afterEach, expect, it, vi } from "vitest";
import { api } from "@/api";
import { advanceSessionGeneration, SessionChangedError } from "@/lib/session-auth";
import { createConsoleQueryClient } from "@/lib/query-client";

afterEach(() => {
  advanceSessionGeneration();
  vi.unstubAllGlobals();
});

it.each([200, 503])("会话切换时旧响应 %s 的正文尚未读取完成，也丢弃旧结果", async (status) => {
  let body!: ReadableStreamDefaultController<Uint8Array>;
  let received!: () => void;
  const responseStarted = new Promise<void>((resolve) => {
    received = resolve;
  });
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      body = controller;
    },
  });
  const response = new Response(stream, { status });
  const readBody = response.json.bind(response);
  vi.spyOn(response, "json").mockImplementation(() => {
    received();
    return readBody();
  });
  vi.stubGlobal("fetch", async () => response);
  const result = api.overview().catch((error: unknown) => error);
  await responseStarted;
  advanceSessionGeneration();
  body.enqueue(new TextEncoder().encode(JSON.stringify({ detail: "旧会话数据" })));
  body.close();
  expect(await result).toBeInstanceOf(SessionChangedError);
});

it("会话切换后旧查询失败不自动重发", async () => {
  const client = createConsoleQueryClient();
  let finish!: (response: Response) => void;
  const fetch = vi.fn(
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve;
      }),
  );
  vi.stubGlobal("fetch", fetch);
  const result = client
    .fetchQuery({
      queryKey: ["old-session"],
      queryFn: ({ signal }) => api.overview(signal),
      retryDelay: 0,
    })
    .catch((error: unknown) => error);
  advanceSessionGeneration();
  finish(Response.json({ detail: "旧请求失败" }, { status: 503 }));
  expect(await result).toBeInstanceOf(SessionChangedError);
  expect(fetch).toHaveBeenCalledTimes(1);
  client.clear();
});

it("新会话发起的请求正常返回数据", async () => {
  advanceSessionGeneration();
  vi.stubGlobal("fetch", async () => Response.json({ account_count: 3 }));
  expect(await api.overview()).toEqual({ account_count: 3 });
});

it("旧请求在会话切换后返回401时不触发新会话过期", async () => {
  const browser = new EventTarget();
  const expired = vi.fn();
  browser.addEventListener("sub2api-console:session-expired", expired);
  vi.stubGlobal("window", browser);
  let finish!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve;
      }),
  );
  const result = api.overview().catch((error: unknown) => error);
  advanceSessionGeneration();
  finish(Response.json({ detail: "旧会话已过期" }, { status: 401 }));
  expect(await result).toBeInstanceOf(SessionChangedError);
  expect(expired).not.toHaveBeenCalled();
});
