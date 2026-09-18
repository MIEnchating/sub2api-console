import type { QueryClient } from "@tanstack/react-query";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { OnboardingCandidate } from "@/api";

import { boundCandidate, renderOnboarding } from "./onboarding-fixture";

let client: QueryClient | undefined;

afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function candidate(id: string, name: string, status = "active"): OnboardingCandidate {
  return {
    ...boundCandidate(status),
    group_id: id,
    group_name: name,
    description: "OpenAI 专线",
    can_create_key: true,
    can_bind_existing_key: false,
    bound: false,
    key_present: false,
    upstream_key_id: null,
    upstream_key_name: null,
    bound_accounts: [],
  };
}

it("第一页搜索站长时显示第二页的专用分组并允许选择本地分组", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const candidates = [
    ...Array.from({ length: 25 }, (_, index) =>
      candidate(String(index + 1), `公开分组 ${index + 1}`),
    ),
    candidate("113", "站长专用分组"),
  ];
  client = renderOnboarding(candidates[0], false, undefined, { candidates });
  await screen.findByRole("combobox", { name: "公开分组 1 本地分组" });
  expect(screen.queryByRole("combobox", { name: "站长专用分组 本地分组" })).not.toBeInTheDocument();

  const user = userEvent.setup();
  await user.type(screen.getByRole("searchbox", { name: "搜索上游分组" }), "站长");
  const groupSelect = screen.getByRole("combobox", { name: "站长专用分组 本地分组" });
  expect(groupSelect).toBeEnabled();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  fireEvent.click(groupSelect);
  fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
  fireEvent.keyDown(groupSelect, { key: "Escape" });
  expect(screen.getByRole("button", { name: "预览 1 项变更" })).toBeEnabled();

  await user.clear(screen.getByRole("searchbox", { name: "搜索上游分组" }));
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByRole("combobox", { name: "站长专用分组 本地分组" })).toHaveTextContent(
    "备用分组",
  );
  expect(screen.getByRole("button", { name: "预览 1 项变更" })).toBeEnabled();
});

it("在第二页搜索仍有多页结果时回到搜索结果第一页", async () => {
  const candidates = Array.from({ length: 21 }, (_, index) =>
    candidate(String(index + 1), `公开分组 ${index + 1}`),
  );
  client = renderOnboarding(candidates[0], false, undefined, { candidates });
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "转到下一页" }));
  await user.type(screen.getByRole("searchbox", { name: "搜索上游分组" }), "公开");

  expect(screen.getByRole("combobox", { name: "公开分组 1 本地分组" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
});

it("搜索未启用分组时显示无匹配提示，关闭启用筛选后显示该分组", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const candidates = [candidate("113", "站长专用分组", "disabled")];
  client = renderOnboarding(candidates[0], false, undefined, { candidates });
  const user = userEvent.setup();
  await user.type(await screen.findByRole("searchbox", { name: "搜索上游分组" }), "站长");

  expect(screen.getByText("没有匹配的上游分组")).toBeInTheDocument();
  await user.click(screen.getByRole("switch", { name: "仅显示启用分组" }));
  expect(screen.getByRole("combobox", { name: "站长专用分组 本地分组" })).toBeInTheDocument();
});
