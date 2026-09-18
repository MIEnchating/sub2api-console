import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
});
const item: OnboardingBindingPreview = {
  id: "first",
  host: "upstream.test",
  upstreamGroupId: "7",
  upstreamGroup: "上游分组",
  platform: "OpenAI",
  multiplier: "1",
  localGroup: "默认",
  concurrency: 1,
  priority: 1,
  status: "待添加",
};
function renderDialog(pending = false, items = [item]) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(
    ["onboarding-model-options", item.host, item.upstreamGroupId],
    ["upstream", "upstream-one", "upstream-two"],
  );
  const submit = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <OnboardingConfirmDialog
        open
        items={items}
        pending={pending}
        onOpenChange={() => {}}
        onConfirm={submit}
      />
    </QueryClientProvider>,
  );
  return submit;
}

it("单账号可通过键盘添加和删除映射，删除后提交默认配置", async () => {
  const user = userEvent.setup();
  const submit = renderDialog();
  screen.getByRole("button", { name: "添加模型映射" }).focus();
  await user.keyboard("{Enter}");
  expect(screen.getByRole("textbox", { name: "请求模型" })).toHaveFocus();
  expect(screen.getByRole("group", { name: "第 1 条模型映射" })).toHaveClass(
    "grid-cols-[minmax(0,1fr)_2rem]",
    "sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2rem]",
  );
  await user.type(screen.getByRole("textbox", { name: "请求模型" }), "alias");
  await user.click(screen.getByRole("combobox", { name: "上游模型" }));
  await user.click(await screen.findByRole("option", { name: "upstream" }));
  const remove = screen.getByRole("button", { name: "删除第 1 条模型映射" });
  remove.focus();
  await user.keyboard("{Enter}");
  expect(screen.queryByRole("textbox", { name: "请求模型" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "确认提交 1 项变更" }));
  expect(submit).toHaveBeenCalledWith({ first: [] }, { first: {} });
});

it("批量新增时分别提交每个账号的模型映射", async () => {
  const user = userEvent.setup();
  const submit = renderDialog(false, [item, { ...item, id: "second", localGroup: "备用" }]);
  for (const [group, target] of [
    ["默认", "upstream-one"],
    ["备用", "upstream-two"],
  ]) {
    const account = within(screen.getByRole("region", { name: `上游分组 → ${group}` }));
    await user.click(account.getByRole("button", { name: "添加模型映射" }));
    await user.type(account.getByRole("textbox", { name: "请求模型" }), "alias");
    await user.click(account.getByRole("combobox", { name: "上游模型" }));
    await user.click(await screen.findByRole("option", { name: target }));
  }
  await user.click(screen.getByRole("button", { name: "确认提交 2 项变更" }));
  expect(submit).toHaveBeenCalledWith(
    { first: [], second: [] },
    { first: { alias: "upstream-one" }, second: { alias: "upstream-two" } },
  );
});

it("未填写上游模型时显示字段错误并阻止提交，修正后提交", async () => {
  const user = userEvent.setup();
  const submit = renderDialog();
  await user.click(screen.getByRole("button", { name: "添加模型映射" }));
  await user.type(screen.getByRole("textbox", { name: "请求模型" }), "alias");
  await user.click(screen.getByRole("button", { name: "确认提交 1 项变更" }));
  expect(await screen.findByText("请输入模型名称")).toBeVisible();
  const target = screen.getByRole("combobox", { name: "上游模型" });
  expect(target).toHaveAttribute("aria-invalid", "true");
  expect(submit).not.toHaveBeenCalled();
  await user.click(target);
  await user.click(await screen.findByRole("option", { name: "upstream" }));
  await user.click(screen.getByRole("button", { name: "确认提交 1 项变更" }));
  expect(submit).toHaveBeenCalledWith({ first: [] }, { first: { alias: "upstream" } });
});

it("提交中禁止添加映射和重复提交", () => {
  renderDialog(true);
  expect(screen.getByRole("button", { name: "添加模型映射" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "正在提交" })).toBeDisabled();
});
