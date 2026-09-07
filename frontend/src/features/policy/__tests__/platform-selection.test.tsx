import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { PolicyScopeEditor, policyDraft } from "@/App";
import type { AccountStatus } from "@/api";
import { account, policy } from "@/features/accounts/__tests__/fixtures";
import { concreteAccountPlatformOptions } from "@/features/accounts/lib/account-labels";

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
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
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function renderScope(accounts: AccountStatus[] = [], platforms: string[] = []) {
  const onChange = vi.fn();
  render(
    <PolicyScopeEditor
      value={policyDraft({ ...policy, advanced_policy: { scope: { platforms } } })}
      onChange={onChange}
      groups={[]}
      accounts={accounts}
      onRestoreControl={() => undefined}
      restorePending={false}
    />,
  );
  return onChange;
}

it("没有账号时仍提供全局平台选项，并保存平台标识", async () => {
  const onChange = renderScope();
  fireEvent.click(screen.getByRole("combobox", { name: "选择参与守护的平台" }));
  for (const option of concreteAccountPlatformOptions) {
    expect(await screen.findByRole("option", { name: option.label })).toBeVisible();
  }
  fireEvent.click(screen.getByRole("option", { name: "OpenAI" }));
  expect(onChange).toHaveBeenCalledWith(
    expect.objectContaining({
      advanced_policy: expect.objectContaining({ scope: { platforms: ["openai"] } }),
    }),
  );
});

it("账号缺少平台时不把上游类型当作平台，并合并实际自定义平台", async () => {
  renderScope([
    { ...account, id: "1", platform: null, upstream_type: "newapi" },
    { ...account, id: "2", platform: "custom-cloud" },
    { ...account, id: "3", platform: "OPENAI" },
  ]);
  fireEvent.click(screen.getByRole("combobox", { name: "选择参与守护的平台" }));
  expect(await screen.findByRole("option", { name: "custom-cloud" })).toBeVisible();
  expect(screen.getAllByRole("option", { name: "OpenAI" })).toHaveLength(1);
  expect(screen.queryByRole("option", { name: /newapi/i })).not.toBeInTheDocument();
});

it("已保存平台使用统一名称，暂未发现的自定义平台仍可取消", async () => {
  renderScope([], ["openai", "retired-cloud"]);
  const trigger = screen.getByRole("combobox", { name: "选择参与守护的平台" });
  expect(trigger).toHaveTextContent("OpenAI");
  fireEvent.click(trigger);
  expect(await screen.findByRole("option", { name: "OpenAI" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  expect(screen.getByRole("option", { name: /retired-cloud.*当前配置/ })).toHaveAttribute(
    "aria-selected",
    "true",
  );
});
