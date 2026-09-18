import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { AnimationRequest, Task } from "@/api";
import { AnimationCheckPanel } from "../animation-check-panel";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

it.each(["animation", "precheck"] as const)(
  "OAuth 账号执行 %s 时先确认范围并仅按稳定 ID 提交",
  async (mode) => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    vi.stubGlobal("EventSource", undefined);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    clients.push(client);
    client.setQueryData(
      ["accounts"],
      [{ id: "41", name: "OAuth 账号", platform: "openai", account_type: "oauth", groups: [] }],
    );
    client.setQueryData(["model-animation", "schedules"], []);
    client.setQueryData(["model-animation", "history"], []);
    const queued: Task = {
      id: "oauth-animation-task",
      skill: "sub2api-model-animation",
      operation: "account-model-animation",
      status: "queued",
      progress: 0,
      message: "已排队",
      result: { account_ids: ["41"], animations: [] },
      created_at: "2026-09-16T00:00:00Z",
      updated_at: "2026-09-16T00:00:00Z",
    };
    const submitted: AnimationRequest[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
        const url = String(input);
        if (url === "/api/model-checks/animations" && init?.method === "POST") {
          submitted.push(JSON.parse(String(init.body)) as AnimationRequest);
          expect(init.credentials).toBe("include");
          return Response.json(queued);
        }
        if (url.includes("/api/tasks/")) return Response.json(queued);
        if (url === "/api/model-checks/animations") return Response.json([queued]);
        if (url.includes("schedules")) return Response.json([]);
        return Response.json({ categories: [] });
      }),
    );
    render(
      <QueryClientProvider client={client}>
        <AnimationCheckPanel />
      </QueryClientProvider>,
    );
    const account = screen.getByRole("checkbox", { name: /检测 OAuth 账号/ });
    expect(account).not.toHaveAttribute("aria-disabled", "true");
    fireEvent.click(account);
    fireEvent.change(screen.getByRole("combobox", { name: "检测模型" }), {
      target: { value: "gpt-6-astra" },
    });
    const startLabel = mode === "precheck" ? "前置检测（1）" : "开始检测（1 个账号）";
    fireEvent.click(screen.getByRole("button", { name: startLabel }));
    const title = mode === "precheck" ? "确认前置检测范围" : "确认动画检测范围";
    const confirm = await screen.findByRole("dialog", { name: title });
    expect(within(confirm).getByText(/OAuth 账号（ID 41）/)).toBeVisible();
    expect(submitted).toHaveLength(0);
    fireEvent.click(within(confirm).getByRole("button", { name: "确认并开始检测" }));
    await waitFor(() =>
      expect(submitted).toEqual([
        {
          targets: [{ account_id: "41", model: "gpt-6-astra" }],
          timeout_seconds: 120,
          ...(mode === "precheck"
            ? { mode: "precheck", precheck_questions: ["candy", "knowledge-cutoff"] }
            : {}),
        },
      ]),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByRole("article", { name: "账号 OAuth 账号" })).toBeVisible();
    expect(screen.queryByLabelText(/Access Token|API Key/)).not.toBeInTheDocument();
  },
);
