import type { QueryClient } from "@tanstack/react-query";
import { fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import type { GroupStatus, OnboardingCandidate } from "@/api";

import { boundCandidate, renderOnboarding } from "./onboarding-fixture";

let client: QueryClient | undefined;

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("仅选择 Composite 本地分组时按 OpenAI 账号类型启用预览", async () => {
  const candidate: OnboardingCandidate = {
    ...boundCandidate("active"),
    group_name: "国模",
    platform: "composite",
    bindable: true,
    can_create_key: true,
    can_bind_existing_key: false,
    bound: false,
    key_present: false,
    upstream_key_id: null,
    upstream_key_name: null,
    bound_accounts: [],
  };
  const compositeGroup: GroupStatus = {
    id: "3",
    name: "国产-平价",
    platform: "composite",
    platforms: ["composite"],
    account_count: 1,
    scheduling_open: 1,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
  };
  client = renderOnboarding(candidate, false, [compositeGroup]);

  fireEvent.click(await screen.findByRole("combobox", { name: "国模 本地分组" }));
  fireEvent.click(await screen.findByRole("option", { name: /国产-平价/ }));
  fireEvent.keyDown(screen.getByRole("combobox", { name: "国模 本地分组" }), {
    key: "Escape",
  });

  expect(screen.getByRole("cell", { name: "OpenAI" })).toBeVisible();
  const preview = screen.getByRole("button", { name: "预览 1 项变更" });
  expect(preview).toBeEnabled();
  fireEvent.click(preview);

  const confirmation = await screen.findByRole("dialog", { name: "确认账号绑定变更" });
  expect(within(confirmation).getByRole("cell", { name: "OpenAI" })).toBeVisible();
});
