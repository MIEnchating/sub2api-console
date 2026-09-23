import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { AnimationResult, Task } from "@/api";
import { AnimationCheckPanel } from "../animation-check-panel";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function finishedTask(): Task {
  const verdicts = ["passed", "not_passed", "inconclusive", "error"] as const;
  const results: AnimationResult[] = verdicts.map((verdict, i) => ({
    account_id: String(i + 1),
    account_name: `账号${i + 1}`,
    model: "gpt-6-astra",
    mode: "precheck",
    status: verdict === "error" ? "failed" : "succeeded",
    request_id: `r-${i}`,
    duration_ms: 10,
    completed_at: "2026-09-15T00:01:00Z",
    precheck: {
      verdict,
      profile_version: "astra-v1",
      questions: [
        {
          id: "candy",
          verdict,
          answer: verdict === "passed" ? "21" : "22",
          request_id: `r-${i}-candy`,
        },
      ],
    },
  }));
  return {
    id: "precheck-1",
    skill: "sub2api-model-animation",
    operation: "account-model-precheck",
    status: "partial",
    progress: 100,
    message: "检测完成",
    result: { animations: results, account_ids: ["1", "2", "3", "4"] },
    created_at: "2026-09-15T00:00:00Z",
    updated_at: "2026-09-15T00:01:00Z",
  };
}

function setup(task?: Task): void {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(
    ["accounts"],
    [1, 2, 3, 4].map((id) => ({
      id: String(id),
      name: `账号${id}`,
      groups: [],
      platform: "openai",
    })),
  );
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], task ? [task] : []);
  if (task) client.setQueryData(["model-animation", "task", task.id], task);
  render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("tab", { name: "前置检测" }));
  fireEvent.change(screen.getByRole("combobox", { name: "检测模型" }), {
    target: { value: "gpt-6-astra" },
  });
}

it("批量前置检测直接开始后展示糖果题结果，并可分别选择通过或降智账号继续动画检测", async () => {
  const task = finishedTask();
  const bodies: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        bodies.push(JSON.parse(String(init.body)));
        return Response.json(task);
      }
      if (String(input).includes("/api/tasks/")) return Response.json(task);
      if (String(input).endsWith("/model-checks/animations")) return Response.json([task]);
      return Response.json({ categories: [] });
    }),
  );
  setup();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "全选账号" }));
  await user.click(screen.getByRole("button", { name: "前置检测（4）" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await waitFor(() =>
    expect(bodies).toEqual([
      {
        mode: "precheck",
        precheck_questions: ["candy"],
        targets: [1, 2, 3, 4].map((id) => ({ account_id: String(id), model: "gpt-6-astra" })),
        timeout_seconds: 120,
      },
    ]),
  );
  const first = screen.getByRole("article", { name: "账号 账号1" });
  await waitFor(() =>
    expect(within(first).getByRole("region", { name: "前置检测结果" })).toHaveTextContent(
      "前置检测通过",
    ),
  );
  await user.click(within(first).getByRole("button", { name: "查看前置检测详情" }));
  const detail = await screen.findByRole("dialog", { name: "前置检测详情" });
  expect(within(detail).getByText("21")).toBeVisible();
  await user.click(within(detail).getByRole("button", { name: "关闭" }));
  await user.click(screen.getByRole("button", { name: "选择通过（1）" }));
  expect(within(first).getByRole("checkbox")).toBeChecked();
  expect(
    within(screen.getByRole("article", { name: "账号 账号2" })).getByRole("checkbox"),
  ).not.toBeChecked();
  await user.click(screen.getByRole("tab", { name: "账号检测" }));
  await user.click(screen.getByRole("button", { name: "开始检测（1 个账号）" }));
  await waitFor(() =>
    expect(bodies[1]).toMatchObject({ targets: [{ account_id: "1", model: "gpt-6-astra" }] }),
  );
  await user.click(screen.getByRole("tab", { name: "前置检测" }));
  await user.click(screen.getByRole("button", { name: "选择降智（1）" }));
  expect(
    within(screen.getByRole("article", { name: "账号 账号2" })).getByRole("checkbox"),
  ).toBeChecked();
  for (const id of [1, 3, 4])
    expect(
      within(screen.getByRole("article", { name: `账号 账号${id}` })).getByRole("checkbox"),
    ).not.toBeChecked();
});

it("切换模型后不能使用前一个模型的前置检测结果选择账号", () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ categories: [] })),
  );
  setup(finishedTask());
  expect(screen.getByRole("button", { name: "选择通过（1）" })).toBeEnabled();
  fireEvent.change(screen.getByRole("combobox", { name: "检测模型" }), {
    target: { value: "another-model" },
  });
  expect(screen.getByRole("button", { name: "选择通过（0）" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "选择降智（0）" })).toBeDisabled();
});

it("取消全选后禁止前置检测，选择单题后确认并只提交该题", async () => {
  const bodies: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        bodies.push(JSON.parse(String(init.body)));
        return Response.json(finishedTask());
      }
      return Response.json([]);
    }),
  );
  setup();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "全选账号" }));
  await user.click(screen.getByRole("button", { name: "选择前置检测题目" }));
  const menu = screen.getByRole("dialog", { name: "前置检测题目" });
  await user.click(within(menu).getByRole("checkbox", { name: "全选" }));
  expect(screen.getByRole("button", { name: "前置检测（4）" })).toBeDisabled();
  await user.click(within(menu).getByRole("checkbox", { name: "糖果题" }));
  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("button", { name: "前置检测（4）" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await waitFor(() =>
    expect(bodies[0]).toMatchObject({ mode: "precheck", precheck_questions: ["candy"] }),
  );
});

it("前置检测运行中切换到账号检测仍锁定忙碌账号，返回后显示原任务状态", async () => {
  const task = finishedTask();
  task.status = "running";
  task.result = { account_ids: ["1"], animations: [] };
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(task)),
  );
  setup(task);
  const user = userEvent.setup();
  expect(screen.getByRole("status", { name: "正在执行前置检测" })).toBeVisible();
  await user.click(screen.getByRole("tab", { name: "账号检测" }));
  const busyAccount = within(screen.getByRole("article", { name: "账号 账号1" })).getByRole(
    "checkbox",
  );
  expect(busyAccount).toHaveAttribute("aria-disabled", "true");
  await user.click(busyAccount);
  expect(busyAccount).not.toBeChecked();
  expect(screen.queryByRole("region", { name: "前置检测结果" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("tab", { name: "前置检测" }));
  expect(screen.getByRole("status", { name: "正在执行前置检测" })).toBeVisible();
});
