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

function setup() {
  const models = completed("models", { models: ["model-a"], steps: [] });
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

it("选择动画测试后默认使用 astra，允许手动输入模型并在成功后安全预览", async () => {
  const view = setup();
  await selectAnimation();
  const model = screen.getByRole("combobox", { name: "选择测试模型" });
  expect(model).toHaveValue("gpt-6-astra");
  await userEvent.clear(model);
  expect(screen.getByRole("button", { name: "开始测试" })).toBeDisabled();
  await userEvent.type(model, "custom-model");
  await userEvent.keyboard("{Escape}");
  const animation = {
    account_id: "",
    account_name: "接入前探活",
    model: "custom-model",
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
      request_model: "custom-model",
      actual_model: "custom-model",
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
      "custom-model",
      "animation",
    ),
  );
  const preview = await screen.findByRole("region", { name: "动画测试预览" });
  expect(preview).toHaveClass("h-64", "overflow-hidden");
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
