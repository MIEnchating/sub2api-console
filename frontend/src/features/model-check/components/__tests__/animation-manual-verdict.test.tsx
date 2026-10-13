import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import type { AnimationResult, Task } from "@/api";
import { collectAnimationTasks } from "../../lib/animation-task-results";
import { AnimationManualVerdictControls } from "../animation-manual-verdict";
import { AnimationAccountCard } from "../animation-account-card";
import { account } from "@/features/accounts/__tests__/fixtures";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
});

function fixture(): Task {
  return {
    id: "animation-1",
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "succeeded",
    progress: 100,
    message: "检测完成",
    created_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:00:00Z",
    result: {
      animations: [
        {
          account_id: "41",
          account_name: "动画账号",
          model: "test-model",
          request_id: "request-1",
          status: "succeeded",
          duration_ms: 1000,
          completed_at: "2026-10-08T00:00:00Z",
          svg: "<svg />",
        },
      ],
    },
  };
}

function taskResult(task: Task): AnimationResult {
  return collectAnimationTasks([task], new Set()).results.get("41")!;
}

function setup(task = fixture(), card = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  client.setQueryData(["model-animation", "task", task.id], task);
  const result = taskResult(task);
  const view = render(
    <QueryClientProvider client={client}>
      <Toaster />
      {card ? (
        <AnimationAccountCard
          account={{ ...account, name: "动画账号", platform: "openai" }}
          result={result}
          retryDisabled={false}
          onRetry={() => undefined}
          checked={false}
          disabled={false}
          schedulesReady
          onToggle={() => undefined}
          onSchedule={() => undefined}
        />
      ) : (
        <AnimationManualVerdictControls result={result} />
      )}
    </QueryClientProvider>,
  );
  return { client, ...view };
}

function saveResponse(task: Task, verdict: string): Task {
  return {
    ...task,
    result: {
      ...task.result,
      animations: (task.result.animations as AnimationResult[]).map((item) => ({
        ...item,
        manual_verdict: { verdict, updated_at: "2026-10-08T01:00:00Z", updated_by: "operator" },
      })),
    },
  };
}

it("卡片手动标记降智时按当前结果保存并显示红色，重新挂载后保留", async () => {
  const user = userEvent.setup();
  const task = fixture();
  const saved = saveResponse(task, "not_passed");
  const fetch = vi.fn(async () => Response.json(saved));
  vi.stubGlobal("fetch", fetch);
  const view = setup(task, true);
  view.client.setQueryData(["model-check-account-statuses"], []);
  const button = screen.getByRole("button", { name: "降智" });
  expect(button).toHaveAttribute("aria-pressed", "false");
  await user.click(button);
  await waitFor(() => expect(button).toHaveAttribute("aria-pressed", "true"));
  expect(view.client.getQueryState(["model-check-account-statuses"])?.isInvalidated).toBe(true);
  expect(button).toHaveClass("text-destructive", "bg-destructive/10");
  expect(fetch).toHaveBeenCalledWith(
    "/api/model-checks/animations/animation-1/manual-verdict",
    expect.objectContaining({
      method: "PUT",
      credentials: "include",
      body: JSON.stringify({ account_id: "41", request_id: "request-1", verdict: "not_passed" }),
    }),
  );
  view.unmount();
  view.client.clear();
  setup(saved);
  expect(screen.getByRole("button", { name: "降智" })).toHaveAttribute("aria-pressed", "true");
});

it("键盘标记不降智时显示绿色，清除后两种结论都未选中", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      return Response.json(saveResponse(fixture(), JSON.parse(String(init?.body)).verdict));
    }),
  );
  setup();
  const button = screen.getByRole("button", { name: "不降智" });
  button.focus();
  await user.keyboard("{Enter}");
  await waitFor(() => expect(button).toHaveAttribute("aria-pressed", "true"));
  expect(button).toHaveClass("text-success", "bg-success/10");
  await user.click(screen.getByRole("button", { name: "清除标记" }));
  await waitFor(() => expect(button).toHaveAttribute("aria-pressed", "false"));
  expect(screen.getByRole("button", { name: "降智" })).toHaveAttribute("aria-pressed", "false");
  expect(screen.queryByRole("button", { name: "清除标记" })).not.toBeInTheDocument();
});

it("保存期间禁用重复操作，保存失败通过悬浮提示反馈并保留原结论", async () => {
  const user = userEvent.setup();
  let finish!: (response: Response) => void;
  const pending = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(() => pending),
  );
  setup(saveResponse(fixture(), "passed"));
  await user.click(screen.getByRole("button", { name: "降智" }));
  expect(screen.getByRole("status")).toHaveTextContent("正在保存标记");
  expect(screen.getByRole("button", { name: "不降智" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "清除标记" })).toBeDisabled();
  await act(async () =>
    finish(Response.json({ code: "internal_error", detail: "保存失败，请重试" }, { status: 500 })),
  );
  expect(await screen.findByText("保存失败，请重试")).toBeVisible();
  expect(screen.getByRole("button", { name: "不降智" })).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByRole("button", { name: "降智" })).toBeEnabled();
  expect(
    within(screen.getByRole("region", { name: "动画手动标记" })).queryByText("保存失败，请重试"),
  ).not.toBeInTheDocument();
});

it("批次尚未结束时禁用标记并说明等待原因", () => {
  setup({ ...fixture(), status: "running" });
  expect(screen.getByRole("button", { name: "降智" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "不降智" })).toBeDisabled();
  expect(screen.getByText("本批检测完成后可标记")).toBeVisible();
});

it("缓存中同一次请求清除标记时更新按钮，不再回退到原有标记", async () => {
  const view = setup(saveResponse(fixture(), "passed"));
  expect(screen.getByRole("button", { name: "不降智" })).toHaveAttribute("aria-pressed", "true");
  act(() => view.client.setQueryData(["model-animation", "task", "animation-1"], fixture()));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "不降智" })).toHaveAttribute("aria-pressed", "false"),
  );
  expect(screen.queryByRole("button", { name: "清除标记" })).not.toBeInTheDocument();
});

it("同一任务的新请求产生结论时，旧请求控件保留自己的标记", async () => {
  const view = setup(saveResponse(fixture(), "passed"));
  const next = saveResponse(fixture(), "not_passed");
  next.status = "running";
  (next.result.animations as AnimationResult[])[0].request_id = "retest-request";
  act(() => view.client.setQueryData(["model-animation", "task", next.id], next));
  await waitFor(() => expect(screen.getByRole("button", { name: "降智" })).toBeDisabled());
  expect(screen.getByRole("button", { name: "不降智" })).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByRole("button", { name: "降智" })).toHaveAttribute("aria-pressed", "false");
});

it("缓存只有运行状态而没有账号结果时仍禁用，终态到达后恢复标记", async () => {
  const view = setup();
  const running: Task = { ...fixture(), status: "running", result: {} };
  act(() => view.client.setQueryData(["model-animation", "task", running.id], running));
  await waitFor(() => expect(screen.getByRole("button", { name: "降智" })).toBeDisabled());
  act(() => view.client.setQueryData(["model-animation", "task", running.id], fixture()));
  await waitFor(() => expect(screen.getByRole("button", { name: "降智" })).toBeEnabled());
  expect(screen.queryByText("本批检测完成后可标记")).not.toBeInTheDocument();
});

it("控件切换账号时读取该账号的缓存结论并按新账号提交", async () => {
  const first = fixture();
  const second = { ...taskResult(first), account_id: "42", request_id: "request-42" };
  const task: Task = {
    ...first,
    result: {
      animations: [
        ...(first.result.animations as AnimationResult[]),
        {
          ...second,
          manual_verdict: {
            verdict: "passed",
            updated_at: first.updated_at,
            updated_by: "operator",
          },
        },
      ],
    },
  };
  const view = setup(task);
  view.rerender(
    <QueryClientProvider client={view.client}>
      <AnimationManualVerdictControls result={second} />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("button", { name: "不降智" })).toHaveAttribute("aria-pressed", "true");
  const fetch = vi.fn(async () => Response.json(task));
  vi.stubGlobal("fetch", fetch);
  await userEvent.setup().click(screen.getByRole("button", { name: "降智" }));
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith(
      "/api/model-checks/animations/animation-1/manual-verdict",
      expect.objectContaining({
        body: JSON.stringify({ account_id: "42", request_id: "request-42", verdict: "not_passed" }),
      }),
    ),
  );
});

it("键盘聚焦清除标记时显示统一提示并保留本次结果的操作说明", async () => {
  const user = userEvent.setup();
  setup(saveResponse(fixture(), "passed"));
  expect(screen.getByRole("region", { name: "动画手动标记" })).toHaveAccessibleDescription(
    "仅标记本次动画结果，重测后需重新判断",
  );
  screen.getByRole("button", { name: "清除标记" }).focus();
  expect(await screen.findByText("清除标记")).toBeVisible();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByText("清除标记")).not.toBeInTheDocument());
});

it("结果缺少稳定任务来源时不提供无法保存的手动标记", () => {
  const result = taskResult(fixture());
  delete result.source_task_id;
  render(<AnimationManualVerdictControls result={result} />);
  expect(screen.queryByRole("region", { name: "动画手动标记" })).not.toBeInTheDocument();
});

it("账号卡片已有动画结果时，降智与不降智仅出现在底部操作栏且保持图标按钮尺寸", () => {
  const view = setup(fixture(), true);
  const actions = screen.getByRole("group", { name: "账号操作" });
  for (const name of ["降智", "不降智"]) {
    const button = within(actions).getByRole("button", { name });
    expect(button).toHaveClass("size-8");
    expect(screen.getAllByRole("button", { name })).toHaveLength(1);
  }
  expect(within(actions).getByRole("button", { name: "自动检测设置" })).toBeVisible();
  expect(screen.queryByText("手动标记")).not.toBeInTheDocument();
  view.unmount();
  view.client.clear();
});
