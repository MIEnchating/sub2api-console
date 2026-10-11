import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { createConsoleQueryClient } from "@/lib/query-client";
import { ModelSyncSettingsCard } from "../model-sync-settings-card";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.unstubAllGlobals();
});

function mount(patterns?: string[]): void {
  client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, refetchOnMount: false } });
  if (patterns) {
    client.setQueryData(["account-model-sync-settings"], { blocked_patterns: patterns });
  }
  render(
    <QueryClientProvider client={client}>
      <ModelSyncSettingsCard />
    </QueryClientProvider>,
  );
}

it("首次读取失败后禁止编辑空配置，重试成功显示服务端规则", async () => {
  const user = userEvent.setup();
  let resolveResponse!: (response: Response) => void;
  const response = new Promise<Response>((resolve) => {
    resolveResponse = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(Response.json({ detail: "读取失败" }, { status: 503 }))
      .mockImplementationOnce(() => response),
  );
  mount();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByRole("textbox", { name: "屏蔽规则" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "保存屏蔽规则" })).not.toBeInTheDocument();
  await user.click(retry);
  expect(screen.getByRole("status", { name: "正在读取全局屏蔽模型" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  await act(async () => resolveResponse(Response.json({ blocked_patterns: ["model-server-*"] })));
  expect(await screen.findByRole("textbox", { name: "屏蔽规则" })).toHaveValue("model-server-*");
  expect(screen.getByRole("button", { name: "保存屏蔽规则" })).toBeDisabled();
});

it("后台读取失败后重试期间保留可编辑草稿，重试成功后保存草稿而非服务端新规则", async () => {
  const user = userEvent.setup();
  let resolveResponse!: (response: Response) => void;
  const response = new Promise<Response>((resolve) => {
    resolveResponse = resolve;
  });
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(Response.json({ detail: "读取失败" }, { status: 503 }))
      .mockImplementationOnce(() => response)
      .mockImplementationOnce(async (_url: string, init?: RequestInit) => {
        const payload: unknown = JSON.parse(String(init?.body));
        writes.push(payload);
        return Response.json(payload);
      }),
  );
  mount(["model-original-*"]);
  const field = screen.getByRole("textbox", { name: "屏蔽规则" });
  await user.clear(field);
  await user.type(field, "model-draft-*");
  await act(async () => client.refetchQueries({ queryKey: ["account-model-sync-settings"] }));
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(field).toHaveValue("model-draft-*");
  expect(screen.getByRole("button", { name: "保存屏蔽规则" })).toBeDisabled();
  await user.click(retry);
  expect(retry).toBeDisabled();
  expect(field).toBeEnabled();
  await user.type(field, "\nmodel-more-*");
  await act(async () => resolveResponse(Response.json({ blocked_patterns: ["model-server-*"] })));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存屏蔽规则" })).toBeEnabled());
  expect(field).toHaveValue("model-draft-*\nmodel-more-*");
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  await waitFor(() =>
    expect(writes).toEqual([
      {
        blocked_patterns: ["model-draft-*", "model-more-*"],
        expected_patterns: ["model-original-*"],
      },
    ]),
  );
});
