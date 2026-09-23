import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";

const endpoint = `https://custom.example.invalid/${"gateway/".repeat(30)}v1`;
const source = {
  endpoint,
  platform: "anthropic",
  model: "requested-model",
  account_id: "custom-record",
};
const clients: ReturnType<typeof createConsoleQueryClient>[] = [];
afterEach(() => {
  for (const client of clients.splice(0)) client.clear();
  vi.restoreAllMocks();
});

async function setup(
  mode: "animation" | "precheck",
  status: Task["status"],
  legacy = false,
): Promise<void> {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));
  const result = {
    ...(legacy ? { account_id: source.account_id, model: source.model } : source),
    account_name: "自定义接口",
    mode,
    response_model: "returned-model",
    status,
    svg: '<svg xmlns="http://www.w3.org/2000/svg"/>',
    request_id: "request-custom",
    duration_ms: 1234,
    completed_at: "2026-09-21T00:00:00Z",
  };
  const task: Task = {
    id: "custom-task",
    skill: "sub2api-model-animation",
    operation: `account-model-${mode}`,
    status,
    message: "检测",
    progress: 0,
    created_at: result.completed_at,
    updated_at: result.completed_at,
    result: {
      targets: [legacy ? { account_id: source.account_id, model: source.model } : source],
      account_ids: [source.account_id],
      animations: status === "succeeded" || status === "failed" ? [result] : [],
    },
  };
  const client = createConsoleQueryClient();
  clients.push(client);
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(["accounts"], []);
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "quality-history"], []);
  client.setQueryData(["model-animation", "history"], [task]);
  client.setQueryData(["model-animation", "task", task.id], task);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(task)),
  );
  render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  await userEvent.click(screen.getByRole("tab", { name: "自定义接口" }));
}

it.each(["animation", "precheck"] as const)(
  "%s 历史记录展示原接口和模型，键盘可打开完整详情",
  async (mode) => {
    await setup(mode, "succeeded");
    const card = screen.getByRole("article", { name: "自定义检测 requested-model" });
    expect(within(card).getByText(endpoint)).toBeVisible();
    expect(within(card).getByText(endpoint)).toHaveClass("wrap-anywhere");
    expect(card).toHaveTextContent("Anthropic");
    expect(card).toHaveTextContent("requested-model");
    const title = mode === "animation" ? "动画检测详情" : "前置检测详情";
    const trigger = within(card).getByRole("button", { name: `查看${title}` });
    trigger.focus();
    await userEvent.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog", { name: title });
    expect(dialog).toHaveTextContent(endpoint);
    expect(dialog).toHaveTextContent("Anthropic");
    expect(dialog).toHaveTextContent("requested-model");
    expect(dialog).toHaveTextContent("returned-model");
    await userEvent.keyboard("{Escape}");
    expect(trigger).toHaveFocus();
  },
);

it.each(["running", "cancelled", "failed"] as const)(
  "自定义任务为 %s 时仍能辨认原接口和请求模型",
  async (status) => {
    await setup("animation", status);
    const card = screen.getByRole("article", { name: "自定义检测 requested-model" });
    expect(card).toHaveTextContent(endpoint);
    expect(card).toHaveTextContent("Anthropic");
    expect(card).toHaveTextContent("requested-model");
  },
);

it("旧记录未保存接口地址时明确显示未记录", async () => {
  await setup("animation", "succeeded", true);
  expect(screen.getByRole("article")).toHaveTextContent("接口地址未记录");
});
