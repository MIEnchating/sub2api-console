import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { GroupsPage } from "@/App";
import type { GroupStatus } from "@/api";
import { policy } from "@/features/accounts/__tests__/fixtures";

let client: QueryClient | undefined;

afterEach(() => {
  client?.clear();
});

function openGroupEditor(override: GroupStatus["override"]) {
  const group: GroupStatus = {
    id: "7",
    name: "codex",
    account_count: 1,
    scheduling_open: 1,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
    override,
  };
  client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["groups"], [group]);
  client.setQueryData(["policy"], policy);
  render(
    <QueryClientProvider client={client}>
      <GroupsPage />
    </QueryClientProvider>,
  );

  fireEvent.click(screen.getByRole("button", { name: "编辑分组" }));
  return within(screen.getByRole("dialog", { name: "编辑分组策略" }));
}

it("分组尚未配置动画倍率时显示温和的推荐值", () => {
  const dialog = openGroupEditor(null);

  expect(dialog.getByRole("spinbutton", { name: "检测通过后：增加分配机会" })).toHaveValue(1.2);
  expect(dialog.getByRole("spinbutton", { name: "检测降智后：减少分配机会" })).toHaveValue(0.7);
});

it("分组已保存中性倍率时不以推荐值覆盖", () => {
  const dialog = openGroupEditor({
    animation_pass_multiplier: 1,
    animation_fail_multiplier: 1,
  });

  expect(dialog.getByRole("spinbutton", { name: "检测通过后：增加分配机会" })).toHaveValue(1);
  expect(dialog.getByRole("spinbutton", { name: "检测降智后：减少分配机会" })).toHaveValue(1);
});

it("旧分组策略缺少动画倍率时保持原有中性值", () => {
  const dialog = openGroupEditor({ enabled: true });

  expect(dialog.getByRole("spinbutton", { name: "检测通过后：增加分配机会" })).toHaveValue(1);
  expect(dialog.getByRole("spinbutton", { name: "检测降智后：减少分配机会" })).toHaveValue(1);
});
