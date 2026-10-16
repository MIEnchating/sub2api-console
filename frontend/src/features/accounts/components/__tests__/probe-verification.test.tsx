import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api, type Task } from "@/api";
import { AccountProbeDialog } from "../account-probe-dialog";

beforeEach(() => {
  const matches = Element.prototype.matches;
  vi.spyOn(Element.prototype, "matches").mockImplementation(function (
    this: Element,
    selector: string,
  ) {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  });
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

function task(id: string, result: Task["result"], status: Task["status"] = "succeeded"): Task {
  return {
    id,
    skill: "onboarding",
    operation: `onboarding-probe-${id}`,
    status,
    progress: 100,
    message: "检测完成",
    result,
    created_at: "",
    updated_at: "",
  };
}

const clients: QueryClient[] = [];

function setup(models: string[] = ["selected-model"]) {
  vi.spyOn(api, "modelCheckCapabilities").mockResolvedValue({
    claude_standards: [],
    sol_models: [],
    public_references: [
      {
        model: "selected-model",
        aliases: ["selected-model"],
        requests: 12,
        methods: ["tokenizer"],
      },
    ],
  });
  const modelsTask = task("models", { models, steps: [] });
  const completed = task(
    "probe",
    {
      steps: [],
      probe_result: {
        status: "completed",
        request_model: "selected-model",
        actual_model: "selected-model",
        message: "请查看报告",
        latency_ms: 10,
        http_status: 0,
      },
      tests: [
        {
          checker: "verification",
          account_name: "接入前探活",
          claimed_model: "selected-model",
          capability: { status: "failed", summary: "答案未通过" },
          authenticity: { status: "inconclusive", summary: "第三方接口" },
          probes: [],
          evidence: [],
          passed: 0,
          capability_total: 7,
          total: 17,
        },
      ],
    },
    "succeeded",
  );
  const start = vi
    .spyOn(api, "startOnboardingProbeTask")
    .mockImplementation(async (action) => (action === "probe" ? completed : modelsTask));
  vi.spyOn(api, "task").mockImplementation(async (id) => (id === "probe" ? completed : modelsTask));
  vi.spyOn(api, "accountCreationSettings").mockImplementation(() => new Promise(() => {}));
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const close = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <AccountProbeDialog
        open
        target={{ kind: "onboarding", host: "upstream.test", groupId: "6", name: "测试分组" }}
        onOpenChange={close}
      />
    </QueryClientProvider>,
  );
  return { start, client, completed, close };
}

it("选择模型检测后沿用当前上游模型，异常检测仍展示共享报告且切换模式清除旧报告", async () => {
  const user = userEvent.setup();
  const fixture = setup();
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await user.click(await screen.findByRole("option", { name: "模型检测" }));
  expect(screen.getByRole("combobox", { name: "选择测试模型" })).toHaveTextContent(
    "selected-model",
  );
  await user.click(screen.getByRole("button", { name: "开始测试" }));
  expect(fixture.start).toHaveBeenLastCalledWith(
    "probe",
    "upstream.test",
    "6",
    "selected-model",
    "verification",
  );
  expect(await screen.findByRole("region", { name: "账号模型检测结果" })).toBeVisible();
  expect(screen.getByText("能力未通过")).toBeVisible();
  await user.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await user.click(await screen.findByRole("option", { name: "常规请求" }));
  await user.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await user.click(await screen.findByRole("option", { name: "模型检测" }));
  expect(screen.queryByRole("region", { name: "账号模型检测结果" })).not.toBeInTheDocument();
  cleanup();
  fixture.client.clear();
});

it("上游没有模型时模型检测保持禁用并允许关闭", async () => {
  const user = userEvent.setup();
  const fixture = setup([]);
  await screen.findByRole("button", { name: "重新获取上游模型" });
  await user.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await user.click(await screen.findByRole("option", { name: "模型检测" }));
  expect(screen.getByRole("button", { name: "开始测试" })).toBeDisabled();
  expect(
    within(screen.getByRole("group", { name: "探活操作" })).getByRole("button", {
      name: "关闭",
    }),
  ).toBeEnabled();
  expect(fixture.start).not.toHaveBeenCalledWith(
    "probe",
    expect.anything(),
    expect.anything(),
    expect.anything(),
    expect.anything(),
  );
  cleanup();
  fixture.client.clear();
});

it("模型检测在生成报告前失败时显示任务原因并保留重试入口", async () => {
  const user = userEvent.setup();
  const fixture = setup();
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await user.click(await screen.findByRole("option", { name: "模型检测" }));
  const failed: Task = {
    ...fixture.completed,
    status: "failed",
    message: "模型 ID 不能包含 API Key，请检查填写内容",
    result: { steps: [], probe_result: fixture.completed.result.probe_result },
  };
  fixture.start.mockResolvedValue(failed);
  vi.mocked(api.task).mockResolvedValue(failed);
  await user.click(screen.getByRole("button", { name: "开始测试" }));
  expect(await screen.findByText(failed.message)).toBeVisible();
  expect(screen.getByRole("button", { name: "重试" })).toBeEnabled();
});

it("模型检测运行时展示逐项过程且不显示进度条，取消后清理临时 Key 再关闭", async () => {
  const user = userEvent.setup();
  const fixture = setup();
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await user.click(await screen.findByRole("option", { name: "模型检测" }));
  const running = {
    ...task(
      "live",
      {
        steps: [],
        completed: 0,
        total: 1,
        phase: "testing",
        active: {
          verification: {
            checker: "verification",
            account_name: "接入前探活",
            claimed_model: "selected-model",
            probes: [],
            total: 17,
            capability_total: 7,
          },
        },
      },
      "running",
    ),
    progress: 23,
    message: "正在检测",
  };
  fixture.start.mockResolvedValue(running);
  vi.mocked(api.task).mockResolvedValue(running);
  const cancel = vi.spyOn(api, "cancelTask").mockImplementation(async () => {
    vi.mocked(api.task).mockResolvedValue({ ...running, status: "cancelled" });
    return { cancelled: true };
  });
  await user.click(screen.getByRole("button", { name: "开始测试" }));
  expect(await screen.findByRole("region", { name: "逐项检测过程" })).toBeVisible();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "取消任务" })).not.toBeInTheDocument();
  expect(screen.queryByRole("navigation", { name: "检测组合" })).not.toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "选择测试模式" })).toBeDisabled();
  expect(screen.getByRole("combobox", { name: "选择测试模型" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "测试中" })).toBeDisabled();
  expect(screen.getByRole("region", { name: "模型检测报告" })).not.toHaveClass("overflow-hidden");
  expect(screen.getByRole("region", { name: "逐项检测过程" })).not.toHaveClass("overflow-auto");
  const cleaned = task("cleaned", { steps: [] });
  fixture.start.mockImplementation(async () => {
    vi.mocked(api.task).mockResolvedValue(cleaned);
    return cleaned;
  });
  await user.click(screen.getByRole("button", { name: "取消并关闭" }));
  await waitFor(() => expect(cancel).toHaveBeenCalledWith("live"));
  await waitFor(() => expect(fixture.close).toHaveBeenCalledWith(false));
  expect(fixture.start).toHaveBeenLastCalledWith(
    "cleanup",
    "upstream.test",
    "6",
    undefined,
    undefined,
  );
});

it("切换所有探活模式时模型和模式始终保持同一行并保留所选模型", async () => {
  const user = userEvent.setup();
  setup();
  vi.spyOn(api, "animationPrompt").mockResolvedValue({
    version: 1,
    prompt: "动画",
    default_id: "bird",
    templates: [{ id: "bird", name: "鹈鹕", prompt: "动画" }],
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  for (const mode of ["常规请求", "流式请求", "动画测试", "糖果题测试", "模型检测"]) {
    await user.click(screen.getByRole("combobox", { name: "选择测试模式" }));
    await user.click(await screen.findByRole("option", { name: mode }));
    expect(screen.getByRole("group", { name: "探活参数" })).toHaveClass(
      "grid-cols-[minmax(0,1fr)_7rem]",
    );
    expect(screen.getByRole("combobox", { name: "选择测试模型" })).toHaveTextContent(
      "selected-model",
    );
  }
});

it("能力题未通过但请求完整时显示检测完成并保留能力异常", async () => {
  const user = userEvent.setup();
  setup();
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await user.click(await screen.findByRole("option", { name: "模型检测" }));
  await user.click(screen.getByRole("button", { name: "开始测试" }));
  expect(await screen.findByText("检测完成 · 1 个组合")).toBeVisible();
  expect(screen.queryByText("检测失败 · 1 个组合")).not.toBeInTheDocument();
  expect(screen.getByText("能力未通过")).toBeVisible();
  expect(screen.getByRole("button", { name: "重试" })).toBeEnabled();
});

it("模型检测统一展示参考预算并提交同一个检测模式", async () => {
  const user = userEvent.setup();
  const fixture = setup();
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  expect(screen.queryByRole("option", { name: "行为指纹" })).not.toBeInTheDocument();
  expect(screen.queryByRole("option", { name: "真伪检测" })).not.toBeInTheDocument();
  await user.click(await screen.findByRole("option", { name: "模型检测" }));
  expect(await screen.findByText(/额外 12 次请求/)).toBeVisible();
  await user.click(screen.getByRole("button", { name: "开始测试" }));
  expect(fixture.start).toHaveBeenLastCalledWith(
    "probe",
    "upstream.test",
    "6",
    "selected-model",
    "verification",
  );
  expect(await screen.findByRole("region", { name: "账号模型检测结果" })).toBeVisible();
});

it("检测项结束但临时 Key 尚在清理时显示收尾，终态后恢复重试按钮", async () => {
  const user = userEvent.setup();
  const fixture = setup();
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await user.click(await screen.findByRole("option", { name: "模型检测" }));
  const finishing = {
    ...fixture.completed,
    status: "running" as const,
    result: {
      ...fixture.completed.result,
      steps: [
        { stage: "request", status: "failed", started_at: "", finished_at: "" },
        { stage: "cleanup_key", status: "running", started_at: "" },
      ],
    },
  };
  fixture.start.mockResolvedValue(finishing);
  vi.mocked(api.task).mockResolvedValue(finishing);
  await user.click(screen.getByRole("button", { name: "开始测试" }));
  expect(await screen.findByRole("button", { name: "正在收尾" })).toBeDisabled();
  const failed = { ...fixture.completed, status: "failed" as const, message: "知识边界请求超时" };
  vi.mocked(api.task).mockResolvedValue(failed);
  await fixture.client.invalidateQueries({ queryKey: ["onboarding-probe-task", "probe"] });
  await waitFor(() => expect(screen.getByRole("button", { name: "重试" })).toBeEnabled());
  expect(screen.queryByRole("button", { name: "测试中" })).not.toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "选择测试模型" })).toBeEnabled();
  await user.click(await screen.findByRole("button", { name: "来源与指纹" }));
  expect(screen.queryByRole("dialog", { name: "检测证据" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "逐项检测结果" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "重试" })).toBeEnabled());
});
