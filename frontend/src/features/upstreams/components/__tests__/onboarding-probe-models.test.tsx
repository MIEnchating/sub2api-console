import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render as renderView,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { api, type Task } from "@/api";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  OnboardingConfirmDialog,
  type OnboardingBindingPreview,
} from "../onboarding-confirm-dialog";

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
  vi.restoreAllMocks();
});
const item: OnboardingBindingPreview = {
  id: "account-one",
  host: "models.test",
  upstreamGroupId: "7",
  upstreamGroup: "分组",
  platform: "OpenAI",
  multiplier: "1",
  localGroup: "默认",
  concurrency: 10,
  priority: 1,
  status: "待添加",
};

const clients: QueryClient[] = [];
function render(element: ReactElement, inheritedModel?: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  if (inheritedModel) {
    client.setQueryDefaults(["policy"], { staleTime: Infinity });
    client.setQueryDefaults(["groups"], { staleTime: Infinity });
    client.setQueryData(["policy"], { available: true, probe_model: inheritedModel });
    client.setQueryData(["groups"], [{ id: "3", name: "默认", override: null }]);
  }
  clients.push(client);
  return renderView(element, {
    wrapper: (props) => <QueryClientProvider client={client}>{props.children}</QueryClientProvider>,
  });
}
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
});

it("获取上游模型后可多选并按当前账号提交，无需手写", async () => {
  const user = userEvent.setup();
  const submit = vi.fn();
  const task: Task = {
    id: "model-options-one",
    skill: "onboarding",
    operation: "onboarding-probe-model-options",
    status: "queued",
    progress: 0,
    message: "",
    result: {},
    created_at: "",
    updated_at: "",
  };
  const start = vi.spyOn(api, "startOnboardingProbeTask").mockResolvedValue(task);
  vi.spyOn(api, "task").mockResolvedValue({
    ...task,
    status: "succeeded",
    result: { models: ["gpt-5.2", "gpt-5.1"] },
  });
  render(
    <OnboardingConfirmDialog
      open
      items={[item]}
      pending={false}
      onOpenChange={vi.fn()}
      onConfirm={submit}
    />,
  );
  const account = screen.getByRole("region", { name: "分组 → 默认" });
  expect(start).not.toHaveBeenCalled();
  await user.click(within(account).getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(start).toHaveBeenCalledWith("model-options", "models.test", "7"));
  const select = within(account).getByRole("combobox", { name: "分组 → 默认 探活模型" });
  await waitFor(() => expect(select).not.toHaveAttribute("aria-disabled", "true"));
  await user.click(select);
  await user.click(await screen.findByRole("option", { name: "gpt-5.2" }));
  await user.click(screen.getByRole("option", { name: "gpt-5.1" }));
  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("button", { name: "确认提交 1 项变更" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      { "account-one": ["gpt-5.2", "gpt-5.1"] },
      { "account-one": {} },
    ),
  );
});

it("模型任务尚未返回时显示获取状态并阻止提交，取消仍可用", async () => {
  let resolveStart!: (task: Task) => void;
  vi.spyOn(api, "startOnboardingProbeTask").mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveStart = resolve;
      }),
  );
  const close = vi.fn();
  render(
    <OnboardingConfirmDialog
      open
      items={[item]}
      pending={false}
      onOpenChange={close}
      onConfirm={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "获取模型" }));
  expect(await screen.findByRole("status", { name: "正在获取上游模型" })).toBeVisible();
  expect(screen.getByRole("button", { name: "获取模型" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "确认提交 1 项变更" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(close).toHaveBeenCalledWith(false);
  cleanup();
  await act(async () => {
    resolveStart({
      id: "detached-task",
      skill: "onboarding",
      operation: "onboarding-probe-model-options",
      status: "queued",
      progress: 0,
      message: "",
      result: {},
      created_at: "",
      updated_at: "",
    });
  });
});

it("获取模型失败后提供重试且仍可留空使用默认模型", async () => {
  vi.spyOn(api, "startOnboardingProbeTask").mockRejectedValue(new Error("上游暂不可用"));
  const submit = vi.fn();
  render(
    <OnboardingConfirmDialog
      open
      items={[item]}
      pending={false}
      onOpenChange={vi.fn()}
      onConfirm={submit}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "获取模型" }));
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeEnabled();
  expect(screen.queryByRole("status", { name: "正在获取上游模型" })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "确认提交 1 项变更" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({ "account-one": [] }, { "account-one": {} }),
  );
});

it("新增账号确认时可输入探活模型并将去重后的模型交给提交", async () => {
  const submit = vi.fn();
  render(
    <OnboardingConfirmDialog
      open
      items={[item]}
      pending={false}
      onOpenChange={vi.fn()}
      onConfirm={submit}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "手动填写" }));
  fireEvent.change(screen.getByRole("textbox", { name: "分组 → 默认 探活模型" }), {
    target: { value: "gpt-5.2\ngpt-5.1\ngpt-5.2" },
  });
  fireEvent.click(screen.getByRole("button", { name: "确认提交 1 项变更" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      { "account-one": ["gpt-5.2", "gpt-5.1"] },
      { "account-one": {} },
    ),
  );
});

it("留空探活模型时使用默认模型并明确不立即发送探活请求", async () => {
  const submit = vi.fn();
  render(
    <OnboardingConfirmDialog
      open
      items={[item]}
      pending={false}
      onOpenChange={vi.fn()}
      onConfirm={submit}
    />,
  );
  expect(screen.getByText(/留空使用默认探活模型/)).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "确认提交 1 项变更" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({ "account-one": [] }, { "account-one": {} }),
  );
});

it("模型名称超长时阻止提交并在模型字段说明原因", async () => {
  const submit = vi.fn();
  render(
    <OnboardingConfirmDialog
      open
      items={[item]}
      pending={false}
      onOpenChange={vi.fn()}
      onConfirm={submit}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "手动填写" }));
  const input = screen.getByRole("textbox", { name: "分组 → 默认 探活模型" });
  fireEvent.change(input, { target: { value: "x".repeat(257) } });
  fireEvent.click(screen.getByRole("button", { name: "确认提交 1 项变更" }));
  await waitFor(() => expect(input).toHaveAttribute("aria-invalid", "true"));
  expect(screen.getByText("每个探活模型名称最多 256 个字符")).toBeVisible();
  expect(submit).not.toHaveBeenCalled();
});

it("仅更新已有绑定时不提供会覆盖模型的新增配置", () => {
  render(
    <OnboardingConfirmDialog
      open
      items={[{ ...item, status: "待更新" }]}
      pending={false}
      onOpenChange={vi.fn()}
      onConfirm={vi.fn()}
    />,
  );
  expect(screen.queryByRole("textbox", { name: "分组 → 默认 探活模型" })).not.toBeInTheDocument();
});

it("同批两个新增账号分别设置模型，提交时不互相覆盖", async () => {
  const submit = vi.fn();
  render(
    <OnboardingConfirmDialog
      open
      items={[item, { ...item, id: "account-two", localGroup: "备用" }]}
      pending={false}
      onOpenChange={vi.fn()}
      onConfirm={submit}
    />,
  );
  screen.getAllByRole("button", { name: "手动填写" }).forEach((button) => fireEvent.click(button));
  fireEvent.change(screen.getByRole("textbox", { name: "分组 → 默认 探活模型" }), {
    target: { value: "gpt-5.2" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: "分组 → 备用 探活模型" }), {
    target: { value: "gpt-5.1" },
  });
  fireEvent.click(screen.getByRole("button", { name: "确认提交 2 项变更" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      { "account-one": ["gpt-5.2"], "account-two": ["gpt-5.1"] },
      { "account-one": {}, "account-two": {} },
    ),
  );
});

it("提交进行中各账号模型不可编辑，确认和取消均不可重复触发", () => {
  render(
    <OnboardingConfirmDialog
      open
      items={[item]}
      pending
      onOpenChange={vi.fn()}
      onConfirm={vi.fn()}
    />,
  );
  expect(screen.getByRole("combobox", { name: "分组 → 默认 探活模型" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByRole("button", { name: "获取模型" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "手动填写" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "正在提交" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeDisabled();
});

it("重新预览不同稳定账号时清除前一账号的模型草稿", () => {
  const callbacks = { onOpenChange: vi.fn(), onConfirm: vi.fn() };
  const view = render(
    <OnboardingConfirmDialog open items={[item]} pending={false} {...callbacks} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "手动填写" }));
  fireEvent.change(screen.getByRole("textbox", { name: "分组 → 默认 探活模型" }), {
    target: { value: "gpt-5.2" },
  });
  view.rerender(
    <OnboardingConfirmDialog
      open
      items={[{ ...item, id: "different-account" }]}
      pending={false}
      {...callbacks}
    />,
  );
  expect(screen.getByRole("combobox", { name: "分组 → 默认 探活模型" })).toHaveTextContent(
    "默认模型",
  );
});

it("切换填写方式使用有选中状态的按钮且保留模型草稿和标准单行高度", () => {
  render(
    <OnboardingConfirmDialog
      open
      items={[item]}
      pending={false}
      onOpenChange={vi.fn()}
      onConfirm={vi.fn()}
    />,
  );
  const manual = screen.getByRole("button", { name: "手动填写" });
  const automatic = screen.getByRole("button", { name: "从列表选择" });
  expect(automatic).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByRole("combobox", { name: "分组 → 默认 探活模型" })).toHaveClass(
    "h-8",
    "min-h-8",
  );
  fireEvent.click(manual);
  const input = screen.getByRole("textbox", { name: "分组 → 默认 探活模型" });
  expect(input).toHaveClass("h-8", "min-h-8", "resize-none", "field-sizing-fixed");
  expect(manual).toHaveAttribute("aria-pressed", "true");
  fireEvent.change(input, { target: { value: "custom-one\ncustom-two" } });
  fireEvent.click(automatic);
  expect(screen.getByRole("combobox", { name: "分组 → 默认 探活模型" })).toHaveTextContent(
    "custom-one",
  );
  fireEvent.click(manual);
  expect(screen.getByRole("textbox", { name: "分组 → 默认 探活模型" })).toHaveValue(
    "custom-one\ncustom-two",
  );
});

it("确认添加时显示继承的具体模型，提交仍留空以跟随后续策略变化", async () => {
  const submit = vi.fn();
  render(
    <OnboardingConfirmDialog
      open
      items={[{ ...item, localGroupIds: ["3"] }]}
      pending={false}
      onOpenChange={vi.fn()}
      onConfirm={submit}
    />,
    "global-model",
  );
  expect(screen.getByRole("note", { name: "继承的探活配置" })).toHaveTextContent(
    "继承全局模型：global-model",
  );
  fireEvent.click(screen.getByRole("button", { name: "确认提交 1 项变更" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith({ "account-one": [] }, { "account-one": {} }),
  );
});
