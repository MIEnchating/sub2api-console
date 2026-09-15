import { QueryObserver } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { api } from "@/api";
import { createConsoleQueryClient } from "../query-client";

const clients: ReturnType<typeof createConsoleQueryClient>[] = [];

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
});

async function readTemplate(): Promise<void> {
  const client = createConsoleQueryClient();
  clients.push(client);
  const observer = new QueryObserver(client, {
    queryKey: ["uptime-kuma", "template-editor", "removed-template"],
    queryFn: () => api.kumaTemplate("removed-template"),
    retryDelay: 0,
  });
  let unsubscribe = (): void => {};
  await new Promise<void>((resolve) => {
    unsubscribe = observer.subscribe((result) => {
      if (result.isError || result.isSuccess) resolve();
    });
  });
  unsubscribe();
}

it.each([400, 401, 403, 404, 422])(
  "查询返回不可通过重试恢复的 HTTP %i 时仅发起一次请求",
  async (status) => {
    const request = vi.fn(async () => Response.json({ detail: "请求不能执行" }, { status }));
    vi.stubGlobal("fetch", request);

    await readTemplate();

    expect(request).toHaveBeenCalledTimes(1);
  },
);

it.each([408, 429, 502])("查询遇到临时 HTTP %i 后通过重试恢复", async (status) => {
  const request = vi
    .fn()
    .mockResolvedValueOnce(Response.json({ detail: "请求暂时不可用" }, { status }))
    .mockResolvedValueOnce(Response.json({ id: "removed-template" }));
  vi.stubGlobal("fetch", request);

  await readTemplate();

  expect(request).toHaveBeenCalledTimes(2);
});

it("查询持续遇到网络故障时最多重试三次", async () => {
  const request = vi.fn().mockRejectedValue(new TypeError("网络连接失败"));
  vi.stubGlobal("fetch", request);

  await readTemplate();

  expect(request).toHaveBeenCalledTimes(4);
});
