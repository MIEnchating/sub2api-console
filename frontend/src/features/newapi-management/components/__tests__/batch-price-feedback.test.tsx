import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";

import { createConsoleQueryClient } from "@/lib/query-client";
import { sessionExpiredMessage } from "@/lib/session-auth";
import { modelPriceCatalogQueryOptions } from "../../lib/model-price-catalog-query";
import { NewAPIModelPrices } from "../model-prices";

const clients: ReturnType<typeof createConsoleQueryClient>[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  toast.dismiss();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function openPreview(): void {
  const client = createConsoleQueryClient();
  clients.push(client);
  render(
    <>
      <Toaster />
      <NewAPIModelPrices
        models={[{ model: "model-a", input_ratio: "1", completion_ratio: "4" }]}
        onLoadManagementPrices={() =>
          client.fetchQuery(modelPriceCatalogQueryOptions("platform-1"))
        }
        onWriteModelPrices={async () => {
          throw new Error("本用例不应提交价格");
        }}
      />
    </>,
  );
  fireEvent.click(screen.getByRole("checkbox", { name: "选择模型 model-a" }));
  fireEvent.click(screen.getByRole("button", { name: "批量同步（1）" }));
}

it("参考价请求失败时全局与弹窗只发出一次悬浮提示，并可原地重试", async () => {
  const notify = vi.spyOn(toast, "error");
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(Response.json({ detail: "价格读取失败" }, { status: 502 }))
      .mockResolvedValueOnce(
        Response.json({
          models: [{ model: "model-a", model_ratio: "0.5", completion_ratio: "4" }],
        }),
      ),
  );
  openPreview();

  expect(await screen.findByText("价格读取失败")).toBeVisible();
  const dialog = screen.getByRole("dialog");
  await waitFor(() => expect(within(dialog).getByRole("button", { name: "取消" })).toBeEnabled());
  expect(notify).toHaveBeenCalledTimes(1);
  expect(within(dialog).queryByText("价格读取失败")).not.toBeInTheDocument();
  expect(within(dialog).getByRole("button", { name: "确认同步 0 个模型" })).toBeDisabled();
  fireEvent.click(within(dialog).getByRole("button", { name: "重新读取" }));
  expect(await screen.findByRole("button", { name: "确认同步 1 个模型" })).toBeEnabled();
});

it("参考价请求遇到登录过期时交由会话边界处理，不重复弹操作错误", async () => {
  const notify = vi.spyOn(toast, "error");
  vi.stubGlobal("fetch", async () => Response.json({}, { status: 401 }));
  openPreview();

  expect(await screen.findByRole("button", { name: "重新读取" })).toBeEnabled();
  expect(notify).not.toHaveBeenCalled();
  expect(screen.queryByText(sessionExpiredMessage)).not.toBeInTheDocument();
});

it("预览读取中显示忙碌提示且允许取消，迟到响应不会重新打开弹窗", async () => {
  let resolveResponse!: (value: Response) => void;
  const response = new Promise<Response>((resolve) => {
    resolveResponse = resolve;
  });
  vi.stubGlobal("fetch", () => response);
  openPreview();

  expect(screen.getByRole("status", { name: "正在准备批量价格预览" })).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await act(async () => {
    resolveResponse(Response.json({ models: [] }));
    await response;
  });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: "选择模型 model-a" })).toBeChecked();
});
