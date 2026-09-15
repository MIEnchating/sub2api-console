import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkbenchPreviewPanel } from "../components/workbench-preview";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
describe("本地RT确认", () => {
  it("待刷新身份的RT条目先展示旋转影响，确认后才创建转换任务", async () => {
    const requests: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (_input, init) => {
        requests.push(init?.body ? (JSON.parse(String(init.body)) as unknown) : null);
        return Response.json({
          id: "rt-task",
          skill: "account-workbench",
          operation: "account-workbench-convert",
          status: "queued",
          result: {},
          created_at: "2026-09-14T00:00:00Z",
          updated_at: "2026-09-14T00:00:00Z",
        });
      }),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    clients.push(client);
    const converted = vi.fn();
    render(
      <QueryClientProvider client={client}>
        <WorkbenchPreviewPanel
          preview={{
            id: "rt-preview",
            scope: "local-export",
            export_only: true,
            target: "",
            expires_at: new Date(Date.now() + 600000).toISOString(),
            check_after_import: false,
            model: "",
            errors: [],
            items: [
              {
                id: "0",
                index: 0,
                name: "待刷新账号",
                email: "",
                plan_type: "",
                template_id: "",
                template_name: "",
                template_revision: 0,
                group_ids: [],
                duplicate: false,
                refresh_required: true,
              },
            ],
          }}
          pending={false}
          onConfirm={() => undefined}
          onDiscard={() => undefined}
          onConverted={converted}
        />
      </QueryClientProvider>,
    );
    expect(screen.getByText("确认后刷新并核对官方身份")).toBeInTheDocument();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "生成私有 JSON 文件" }));
    expect(screen.getByRole("dialog", { name: "确认生成私有账号文件" })).toHaveTextContent(
      "旧令牌可能失效",
    );
    expect(requests).toEqual([]);
    await user.click(screen.getByRole("button", { name: "创建私有转换任务" }));
    await waitFor(() => expect(converted).toHaveBeenCalledOnce());
    expect(requests).toEqual([{ preview_id: "rt-preview", confirmed: true }]);
  });
});
