import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
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
function setup(): void {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <AnimationQualityExport />
    </QueryClientProvider>,
  );
}
it("样本为空时提示并不下载空文件", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ items: [] })),
  );
  setup();
  await user.click(screen.getByRole("button", { name: "导出标注样本" }));
  expect(await screen.findByText("暂无已标注样本")).toBeVisible();
  expect(screen.queryByText("本轮全部样本已导出")).not.toBeInTheDocument();
});
it("分批下载后使用后端游标，最后一批提示完成", async () => {
  const user = userEvent.setup();
  const urls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      urls.push(url);
      return Response.json({
        items: [{ id: "sample" }],
        next_cursor: urls.length === 1 ? "cursor-1" : undefined,
      });
    }),
  );
  const create = vi.fn(() => "blob:test");
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = create;
      static revokeObjectURL = vi.fn();
    },
  );
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
  setup();
  await user.click(screen.getByRole("button", { name: "导出标注样本" }));
  await user.click(await screen.findByRole("button", { name: "导出下一批样本" }));
  expect(await screen.findByText("本轮全部样本已导出")).toBeVisible();
  expect(urls[1]).toContain("after=cursor-1&limit=50");
  expect(create).toHaveBeenCalledTimes(2);
});
