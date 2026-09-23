import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { DetectionTask } from "@/api";
import { DetectionTaskPanel } from "../detection-task-panel";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});
const plan: DetectionTask = {
  id: "plan-1",
  version: 2,
  name: "每日检测",
  group_ids: ["7"],
  model: "test-model",
  precheck: true,
  terminal: true,
  terminal_rounds: 3,
  automatic: false,
  schedule_type: "daily",
  daily_times: ["09:00", "20:00"],
  timezone: "Asia/Shanghai",
  interval_minutes: 60,
  timeout_seconds: 120,
};
function setup(values: DetectionTask[] = [plan], failSave = false) {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["model-detection-tasks"], values);
  client.setQueryData(["groups"], [{ id: "7", name: "测试分组" }]);
  const requests: { url: string; method?: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({
        url: String(input),
        method: init?.method,
        body: init?.body ? JSON.parse(String(init.body)) : null,
      });
      if (init?.method === "PUT") {
        if (failSave) return Response.json({ detail: "版本已变化，请刷新" }, { status: 422 });
        return Response.json([JSON.parse(String(init.body))]);
      }
      if (init?.method === "DELETE") return Response.json([]);
      if (String(input).includes("/run") || String(input).includes("/api/tasks/"))
        return Response.json({
          id: "run-1",
          status: "queued",
          progress: 0,
          message: "等待执行",
          result: {},
          created_at: "2026-09-23T00:00:00Z",
        });
      return Response.json(values);
    }),
  );
  render(
    <QueryClientProvider client={client}>
      <DetectionTaskPanel />
    </QueryClientProvider>,
  );
  return requests;
}
it("编辑分组任务可设置多轮终端和多个自动时间并保留版本", async () => {
  const requests = setup();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "编辑" }));
  const dialog = screen.getByRole("dialog", { name: "编辑检测任务" });
  expect(within(dialog).getByRole("checkbox", { name: "前置检测" })).toBeChecked();
  expect(within(dialog).getByLabelText("每天检测时间 2（北京时间）")).toHaveValue("20:00");
  fireEvent.change(within(dialog).getByRole("spinbutton", { name: "终端检测轮数" }), {
    target: { value: "5" },
  });
  await user.click(within(dialog).getByRole("checkbox", { name: "自动检测" }));
  await user.click(within(dialog).getByRole("button", { name: "保存任务" }));
  const confirm = screen.getByRole("dialog", { name: "确认自动检测任务" });
  expect(confirm).toHaveTextContent("测试分组");
  expect(requests.filter((item) => item.method === "PUT")).toHaveLength(0);
  await user.click(within(confirm).getByRole("button", { name: "确认保存并开启" }));
  await waitFor(() =>
    expect(requests.find((item) => item.method === "PUT")?.body).toMatchObject({
      id: "plan-1",
      version: 2,
      group_ids: ["7"],
      model: "test-model",
      terminal_rounds: 5,
      automatic: true,
      daily_times: ["09:00", "20:00"],
    }),
  );
});
it("新任务未选分组不能保存且可以关闭表单", async () => {
  const requests = setup([]);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "新增任务" }));
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  expect(await screen.findByText("请选择至少一个分组")).toBeVisible();
  expect(requests.filter((item) => item.method === "PUT")).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("立即执行使用保存版本直接启动且不自动弹出详情", async () => {
  const requests = setup();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "立即执行" }));
  await waitFor(() =>
    expect(requests.find((item) => item.method === "POST")).toMatchObject({
      url: "/api/model-checks/detection-tasks/plan-1/run",
      body: { version: 2 },
    }),
  );
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("删除任务需确认并使用版本校验", async () => {
  const requests = setup();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "删除" }));
  expect(requests.filter((item) => item.method === "DELETE")).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "确认删除" }));
  await waitFor(() =>
    expect(screen.queryByRole("article", { name: "检测任务 每日检测" })).not.toBeInTheDocument(),
  );
  expect(requests.find((item) => item.method === "DELETE")?.body).toEqual({ version: 2 });
});

it("保存失败时保留编辑内容并允许修改后重试", async () => {
  const requests = setup([plan], true);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "编辑" }));
  const name = screen.getByRole("textbox", { name: "任务名称" });
  await user.clear(name);
  await user.type(name, "保留我的修改");
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() => expect(requests.filter((item) => item.method === "PUT")).toHaveLength(1));
  expect(screen.getByRole("dialog", { name: "编辑检测任务" })).toBeVisible();
  expect(name).toHaveValue("保留我的修改");
  await waitFor(() => expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled());
});

it("运行中的任务禁止重复启动和删除，仍可打开详情取消", async () => {
  setup([{ ...plan, running: true, last_task_id: "run-1" }]);
  const user = userEvent.setup();
  expect(screen.getByRole("button", { name: "立即执行" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "删除" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "运行详情" }));
  expect(await screen.findByRole("button", { name: "取消任务" })).toBeEnabled();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
});
