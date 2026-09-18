import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api, type Task } from "@/api";
import {
  OnboardingConfirmDialog,
  type OnboardingBindingPreview,
} from "../onboarding-confirm-dialog";

let client: QueryClient;
const item: OnboardingBindingPreview = {
  id: "one",
  host: "models.test",
  upstreamGroupId: "7",
  upstreamGroup: "上游",
  platform: "OpenAI",
  multiplier: "1",
  localGroup: "默认",
  concurrency: 1,
  priority: 1,
  status: "待添加",
};
beforeEach(() => {
  const matches = Element.prototype.matches;
  vi.spyOn(Element.prototype, "matches").mockImplementation(function (
    this: Element,
    selector: string,
  ) {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  });
  const task: Task = {
    id: "models-one",
    skill: "onboarding",
    operation: "model-options",
    status: "succeeded",
    progress: 100,
    message: "",
    result: { models: ["gpt-5", "gpt-5.2"] },
    created_at: "",
    updated_at: "",
  };
  vi.spyOn(api, "startOnboardingProbeTask").mockResolvedValue(task);
  vi.spyOn(api, "task").mockResolvedValue(task);
});
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
});
function renderDialog(items = [item]) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const submit = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <OnboardingConfirmDialog
        open
        items={items}
        pending={false}
        onOpenChange={() => {}}
        onConfirm={submit}
      />
    </QueryClientProvider>,
  );
  return submit;
}

it("获取一次后选择上游模型会回显两侧名称，请求模型可改且不会被再次选择覆盖", async () => {
  const user = userEvent.setup();
  const submit = renderDialog();
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await user.click(screen.getByRole("button", { name: "添加模型映射" }));
  const target = screen.getByRole("combobox", { name: "上游模型" });
  await waitFor(() => expect(target).toBeEnabled());
  await user.click(target);
  await user.click(await screen.findByRole("option", { name: "gpt-5" }));
  const source = screen.getByRole("textbox", { name: "请求模型" });
  expect(source).toHaveValue("gpt-5");
  expect(target).toHaveTextContent("gpt-5");
  await user.clear(source);
  await user.type(source, "alias");
  await user.click(target);
  await user.click(await screen.findByRole("option", { name: "gpt-5.2" }));
  expect(source).toHaveValue("alias");
  await user.click(screen.getByRole("button", { name: "确认提交 1 项变更" }));
  expect(submit).toHaveBeenCalledWith({ one: [] }, { one: { alias: "gpt-5.2" } });
  expect(api.startOnboardingProbeTask).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole("button", { name: "重新获取" })).not.toBeInTheDocument();
});

it("同上游分组的两个账号及探活选择共用一次获取的模型列表", async () => {
  const user = userEvent.setup();
  renderDialog([item, { ...item, id: "two", localGroup: "备用" }]);
  await user.click(screen.getAllByRole("button", { name: "获取模型" })[0]);
  for (const group of ["默认", "备用"]) {
    const region = within(screen.getByRole("region", { name: `上游 → ${group}` }));
    await user.click(region.getByRole("button", { name: "添加模型映射" }));
    const target = region.getByRole("combobox", { name: "上游模型" });
    await waitFor(() => expect(target).toBeEnabled());
    await user.click(target);
    await user.click(await screen.findByRole("option", { name: "gpt-5" }));
    expect(region.getByRole("textbox", { name: "请求模型" })).toHaveValue("gpt-5");
    await user.click(region.getByRole("combobox", { name: `上游 → ${group} 探活模型` }));
    expect(await screen.findByRole("option", { name: "gpt-5.2" })).toBeVisible();
    await user.keyboard("{Escape}");
  }
  expect(api.startOnboardingProbeTask).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole("button", { name: "获取模型" })).not.toBeInTheDocument();
});
