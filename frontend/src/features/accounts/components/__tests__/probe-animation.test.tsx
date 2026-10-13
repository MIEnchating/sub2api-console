import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Task } from "@/api";
import { AccountProbeDialog } from "../account-probe-dialog";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

function completed(id: string, result: Task["result"]): Task {
  return {
    id,
    skill: "onboarding",
    operation: "onboarding-probe",
    status: "succeeded",
    progress: 0,
    message: "完成",
    created_at: "",
    updated_at: "",
    result,
  };
}

function setup(modelOptions = ["model-a"]) {
  const models = completed("models", { models: modelOptions, steps: [] });
  const start = vi.spyOn(api, "startOnboardingProbeTask").mockResolvedValue(models);
  vi.spyOn(api, "task").mockResolvedValue(models);
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
        target={{
          kind: "onboarding",
          host: "upstream.test",
          groupId: "6",
          name: "添加账号",
          platform: "openai",
        }}
        onOpenChange={close}
      />
    </QueryClientProvider>,
  );
  return { start, close };
}

async function selectAnimation(): Promise<void> {
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  await userEvent.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await userEvent.click(await screen.findByRole("option", { name: "动画测试" }));
}

it("选择动画测试后保留模型下拉选择并在成功后安全预览", async () => {
  const view = setup();
  await selectAnimation();
  const model = screen.getByRole("combobox", { name: "选择测试模型" });
  expect(model.tagName).toBe("BUTTON");
  expect(model).toHaveTextContent("model-a");
  const animation = {
    account_id: "",
    account_name: "接入前探活",
    model: "model-a",
    request_id: "animation-request",
    status: "succeeded",
    duration_ms: 20,
    completed_at: "2026-10-09T00:00:00Z",
    html: "<!DOCTYPE html><html><body><svg><circle r='10'/></svg></body></html>",
    prompt: "统一动画提示词",
    reasoning_effort: "low",
  };
  const result = completed("animation", {
    steps: [],
    probe_result: {
      status: "passed",
      message: "动画测试完成",
      request_model: "model-a",
      actual_model: "model-a",
      latency_ms: 20,
      http_status: 0,
      animation,
    },
  });
  view.start.mockResolvedValue(result);
  vi.mocked(api.task).mockResolvedValue(result);
  await userEvent.click(screen.getByRole("button", { name: "开始测试" }));
  await waitFor(() =>
    expect(view.start).toHaveBeenLastCalledWith(
      "probe",
      "upstream.test",
      "6",
      "model-a",
      "animation",
    ),
  );
  const preview = await screen.findByRole("region", { name: "动画测试预览" });
  expect(preview).toHaveClass("aspect-[4/3]", "overflow-hidden");
  await userEvent.click(await within(preview).findByRole("button", { name: /放大查看/ }));
  const dialog = within(await screen.findByRole("dialog", { name: "动画预览" }));
  const frame = dialog.getByLabelText(/生成的.*动画/);
  expect(frame).toHaveAttribute("sandbox", "allow-scripts");
  expect(frame.getAttribute("srcdoc")).toContain("connect-src 'none'");
  await userEvent.click(dialog.getByRole("tab", { name: "提示词" }));
  expect(dialog.getByRole("tabpanel", { name: "提示词" })).toHaveTextContent("统一动画提示词");
});

it("动画运行期间禁用参数和重复提交，取消后等待临时 Key 清理再关闭", async () => {
  const view = setup();
  await selectAnimation();
  const running = { ...completed("animation", { steps: [] }), status: "running" as const };
  view.start.mockResolvedValue(running);
  vi.mocked(api.task).mockResolvedValue(running);
  const cancel = vi.spyOn(api, "cancelTask").mockImplementation(async () => {
    vi.mocked(api.task).mockResolvedValue({ ...running, status: "cancelled" });
    return { cancelled: true };
  });
  await userEvent.click(screen.getByRole("button", { name: "开始测试" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "测试中" })).toBeDisabled());
  expect(screen.getByRole("combobox", { name: "选择测试模式" })).toBeDisabled();
  expect(screen.getByRole("combobox", { name: "选择测试模型" })).toBeDisabled();
  expect(screen.getByRole("status", { name: "正在生成动画，请稍候" })).toBeVisible();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  const cleaned = completed("cleanup", { steps: [] });
  view.start.mockImplementation(async () => {
    vi.mocked(api.task).mockResolvedValue(cleaned);
    return cleaned;
  });
  await userEvent.click(screen.getByRole("button", { name: "取消并关闭" }));
  await waitFor(() => expect(cancel).toHaveBeenCalledWith("animation"));
  await waitFor(() => expect(view.close).toHaveBeenCalledWith(false));
  expect(view.start).toHaveBeenLastCalledWith(
    "cleanup",
    "upstream.test",
    "6",
    undefined,
    undefined,
  );
});

it("切换模式和刷新模型后保留手动选择的下拉项，键盘仍可操作", async () => {
  const view = setup(["model-a", "model-b"]);
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  const selector = screen.getByRole("combobox", { name: "选择测试模型" });
  selector.focus();
  await userEvent.keyboard("{ArrowDown}");
  await userEvent.click(await screen.findByRole("option", { name: "model-b" }));
  await selectAnimation();
  expect(selector).toHaveTextContent("model-b");
  const refreshed = completed("models-refresh", { models: ["model-a", "model-b"], steps: [] });
  view.start.mockResolvedValue(refreshed);
  vi.mocked(api.task).mockResolvedValue(refreshed);
  await userEvent.click(screen.getByRole("button", { name: "重新获取上游模型" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  expect(selector).toHaveTextContent("model-b");
  await userEvent.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await userEvent.click(await screen.findByRole("option", { name: "糖果题测试" }));
  expect(selector).toHaveTextContent("model-b");
});

it.each(["21", "22"])("糖果题返回 %s 时展示回答和判定，重试沿用所选模型", async (answer) => {
  const view = setup();
  await waitFor(() => expect(screen.getByRole("button", { name: "开始测试" })).toBeEnabled());
  await userEvent.click(screen.getByRole("combobox", { name: "选择测试模式" }));
  await userEvent.click(await screen.findByRole("option", { name: "糖果题测试" }));
  const passed = answer === "21";
  const message = passed ? "糖果题测试通过" : "糖果题测试未通过：回答不符合预期 21，可重试核对";
  const task = completed("candy", {
    steps: [],
    probe_result: {
      status: passed ? "passed" : "failed",
      message,
      request_model: "model-a",
      actual_model: "actual-model",
      response_text: answer,
      latency_ms: 100,
      http_status: 0,
    },
  });
  view.start.mockResolvedValue(task);
  vi.mocked(api.task).mockResolvedValue(task);
  await userEvent.click(screen.getByRole("button", { name: "开始测试" }));
  expect(await screen.findByText(answer, { exact: true })).toBeVisible();
  if (!passed) expect(screen.getByText(message)).toBeVisible();
  expect(screen.queryByRole("region", { name: "动画展示区域" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "重试" }));
  await waitFor(() =>
    expect(view.start).toHaveBeenLastCalledWith("probe", "upstream.test", "6", "model-a", "candy"),
  );
});

it("已选模型从刷新结果消失时要求重新选择，不能自动换用其他模型", async () => {
  const view = setup();
  await selectAnimation();
  const refreshed = completed("models-new", { models: ["different-model"], steps: [] });
  view.start.mockResolvedValue(refreshed);
  vi.mocked(api.task).mockResolvedValue(refreshed);
  await userEvent.click(screen.getByRole("button", { name: "重新获取上游模型" }));
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "选择测试模型" })).toHaveTextContent(
      "选择上游模型",
    ),
  );
  expect(screen.getByRole("button", { name: "开始测试" })).toBeDisabled();
});

it("动画生成失败时展示实际原因，保留重试入口且不显示预览", async () => {
  const view = setup();
  await selectAnimation();
  const failed = {
    ...completed("animation", {
      steps: [],
      probe_result: {
        status: "failed",
        message: "动画包含外部资源，请重试",
        request_model: "gpt-6-astra",
        actual_model: "",
        latency_ms: 0,
        http_status: 0,
      },
    }),
    status: "failed" as const,
    message: "动画包含外部资源，请重试",
  };
  view.start.mockResolvedValue(failed);
  vi.mocked(api.task).mockResolvedValue(failed);
  await userEvent.click(screen.getByRole("button", { name: "开始测试" }));
  expect(await screen.findByText("动画包含外部资源，请重试")).toBeVisible();
  expect(screen.getByRole("button", { name: "重试" })).toBeEnabled();
  expect(screen.queryByRole("region", { name: "动画测试预览" })).not.toBeInTheDocument();
});
