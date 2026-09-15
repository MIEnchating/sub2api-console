import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { WorkbenchPreview } from "@/api";
import { WorkbenchPreviewPanel } from "../components/workbench-preview";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
});

describe("重新处理账号影响范围", () => {
  it("重新检测和只读核对按实际动作展示且不计入更新凭据数量", async () => {
    const preview: WorkbenchPreview = {
      id: "retry-preview",
      expires_at: new Date(Date.now() + 600000).toISOString(),
      target: "https://sub2api.example.test",
      check_after_import: true,
      model: "test-model",
      errors: [],
      items: [
        {
          id: "7",
          index: 7,
          name: "隔离账号",
          email: "",
          plan_type: "plus",
          template_id: "team",
          template_name: "团队配置",
          template_revision: 2,
          group_ids: ["6"],
          duplicate: true,
          account_id: "42",
          action: "check",
        },
        {
          id: "8",
          index: 8,
          name: "已启用账号",
          email: "",
          plan_type: "plus",
          template_id: "",
          template_name: "",
          template_revision: 0,
          group_ids: [],
          duplicate: true,
          account_id: "43",
          action: "reconcile",
        },
      ],
    };
    const user = userEvent.setup();
    client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <WorkbenchPreviewPanel
          preview={preview}
          pending={false}
          onConfirm={() => undefined}
          onDiscard={() => undefined}
        />
      </QueryClientProvider>,
    );
    expect(screen.getByRole("cell", { name: "重新检测（ID 42）" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "只读核对（ID 43）" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "确认重新处理 2 个账号" }));
    const dialog = screen.getByRole("dialog", { name: "确认重新处理账号" });
    expect(dialog).toHaveTextContent("更新凭据 0 个");
    expect(dialog).toHaveTextContent("重新检测 1 个（ID：42）");
    expect(dialog).toHaveTextContent("只读核对 1 个（ID：43）");
    expect(dialog).toHaveTextContent("只读核对不执行模型调用");
    expect(within(dialog).getByRole("button", { name: "创建重试任务" })).toBeEnabled();
  });
});
