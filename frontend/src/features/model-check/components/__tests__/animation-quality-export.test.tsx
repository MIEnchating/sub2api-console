import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast, Toaster } from "sonner";
import { AnimationQualityExport } from "../animation-quality-export";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function setup(): ReturnType<typeof render> {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <Toaster />
      <AnimationQualityExport />
    </QueryClientProvider>,
  );
}
function downloads(): { create: ReturnType<typeof vi.fn>; click: ReturnType<typeof vi.spyOn> } {
  const create = vi.fn<(blob: Blob) => string>().mockReturnValue("blob:test");
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = create;
      static revokeObjectURL = vi.fn();
    },
  );
  const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
  return { create, click };
}
it("全部账号均无样本时提示且不下载空文件", async () => {
  const download = downloads();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ items: [] })),
  );
  setup();
  await userEvent.setup().click(screen.getByRole("button", { name: "导出全部样本" }));
  expect(await screen.findByText("暂无已收录样本")).toBeVisible();
  expect(download.click).not.toHaveBeenCalled();
});
it("一次点击按游标读取多个账号的全部分页并合并为一个文件", async () => {
  const urls: string[] = [];
  const first = { id: "sample-a", account_id: "41", html: "原文甲" };
  const second = { id: "sample-b", account_id: "42", html: "原文乙" };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      urls.push(url);
      return Response.json(
        urls.length === 1 ? { items: [first], next_cursor: "cursor-1" } : { items: [second] },
      );
    }),
  );
  const download = downloads();
  setup();
  const button = screen.getByRole("button", { name: "导出全部样本" });
  button.focus();
  await userEvent.setup().keyboard("{Enter}");
  expect(await screen.findByText("已导出全部 2 条样本")).toBeVisible();
  expect(urls).toEqual([
    "/api/model-checks/animation-quality/samples?after=&limit=50",
    "/api/model-checks/animation-quality/samples?after=cursor-1&limit=50",
  ]);
  expect(download.create).toHaveBeenCalledTimes(1);
  expect(download.click).toHaveBeenCalledTimes(1);
  const reader = new FileReader();
  const content = new Promise<string>((resolve) => {
    reader.onload = () => resolve(String(reader.result));
  });
  reader.readAsText(download.create.mock.calls[0][0]);
  expect(JSON.parse(await content)).toEqual({ items: [first, second] });
  expect(screen.queryByRole("button", { name: /下一批/ })).not.toBeInTheDocument();
});
it("后续分页读取失败时不下载残缺文件，重试从全部样本起点开始", async () => {
  const cursors: string[] = [];
  let fail = true;
  const download = downloads();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      const cursor = new URL(url, "http://console.test").searchParams.get("after") ?? "";
      cursors.push(cursor);
      if (cursor && fail) return Response.json({ detail: "样本读取失败，请重试" }, { status: 503 });
      return Response.json({
        items: [{ id: cursor || "first" }],
        next_cursor: cursor ? undefined : "second",
      });
    }),
  );
  setup();
  await userEvent.setup().click(screen.getByRole("button", { name: "导出全部样本" }));
  expect(await screen.findByText(/样本读取失败，请重试/)).toBeVisible();
  expect(download.click).not.toHaveBeenCalled();
  fail = false;
  await userEvent.setup().click(screen.getByRole("button", { name: "导出全部样本" }));
  expect(await screen.findByText("已导出全部 2 条样本")).toBeVisible();
  expect(cursors).toEqual(["", "second", "", "second"]);
  expect(download.click).toHaveBeenCalledTimes(1);
});
it("读取样本期间显示忙碌且禁止重复点击，离开页面取消读取而不下载", async () => {
  let signal: AbortSignal | undefined;
  let finish!: (response: Response) => void;
  const download = downloads();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      signal = init?.signal ?? undefined;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }),
  );
  const view = setup();
  await userEvent.setup().click(screen.getByRole("button", { name: "导出全部样本" }));
  expect(screen.getByRole("button", { name: "正在导出全部样本" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "正在导出全部样本" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  view.unmount();
  expect(signal?.aborted).toBe(true);
  await act(async () => finish(Response.json({ items: [{ id: "late" }] })));
  expect(download.click).not.toHaveBeenCalled();
});
it("后端返回重复游标时停止并提示重试，不无限读取或下载不完整样本", async () => {
  const download = downloads();
  const fetcher = vi.fn(async () =>
    Response.json({ items: [{ id: "sample" }], next_cursor: "same" }),
  );
  vi.stubGlobal("fetch", fetcher);
  setup();
  await userEvent.setup().click(screen.getByRole("button", { name: "导出全部样本" }));
  expect(await screen.findByText(/样本分页异常，请重新导出/)).toBeVisible();
  expect(fetcher).toHaveBeenCalledTimes(2);
  expect(download.click).not.toHaveBeenCalled();
  await waitFor(() => expect(screen.getByRole("button", { name: "导出全部样本" })).toBeEnabled());
});
