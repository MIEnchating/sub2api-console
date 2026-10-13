import type { QueryClient } from "@tanstack/react-query";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { api, type OnboardingCandidate, type Task } from "@/api";

import { boundCandidate, renderOnboarding } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function unboundCandidate(): OnboardingCandidate {
  return {
    ...boundCandidate("active"),
    group_name: "国模特价",
    platform: null,
    bindable: true,
    can_create_key: true,
    can_bind_existing_key: false,
    bound: false,
    key_present: false,
    upstream_key_id: null,
    upstream_key_name: null,
    bound_accounts: [],
  };
}

it("从统计变化进入 New API 添加账号且未选本地分组时可用键盘打开探活并获取模型", async () => {
  const user = userEvent.setup();
  const task: Task = {
    id: "probe-models",
    skill: "onboarding",
    operation: "onboarding-probe-models",
    status: "succeeded",
    progress: 100,
    message: "完成",
    result: { models: ["test-model"], steps: [] },
    created_at: "",
    updated_at: "",
  };
  vi.spyOn(api, "startOnboardingProbeTask").mockResolvedValue(task);
  vi.spyOn(api, "task").mockResolvedValue(task);
  vi.spyOn(api, "accountCreationSettings").mockResolvedValue({
    default: {
      models: [],
      concurrency: 10,
      load_factor: null,
      priority: 1,
      pool_mode: false,
      pool_mode_retry_count: 3,
      pool_mode_retry_status_codes: [],
    },
    groups: [],
    platform_probe_models: {},
  });
  client = renderOnboarding(unboundCandidate(), true, undefined, {
    history: "overview",
    upstreamType: "newapi",
  });

  const probe = await screen.findByRole("button", { name: /探活/ });
  expect(probe).toBeEnabled();
  expect(screen.getByRole("button", { name: "预览添加账号" })).toBeDisabled();
  probe.focus();
  await user.keyboard("{Enter}");

  const dialog = await screen.findByRole("dialog", { name: "测试账号连接" });
  await waitFor(() =>
    expect(within(dialog).getByRole("combobox", { name: "选择测试模型" })).toHaveTextContent(
      "test-model",
    ),
  );
  expect(api.startOnboardingProbeTask).toHaveBeenCalledWith(
    "models",
    "api.example.test",
    "7",
    undefined,
    undefined,
  );
});

it.each([true, false])("本地分组为空时仍允许探活（直达分组：%s）", async (directGroup) => {
  client = renderOnboarding(unboundCandidate(), directGroup, [], { upstreamType: "newapi" });
  expect(await screen.findByRole("button", { name: /探活/ })).toBeEnabled();
});

it.each([true, false])(
  "上游不能创建 Key 且无绑定时禁用探活并说明原因（直达分组：%s）",
  async (directGroup) => {
    client = renderOnboarding(
      {
        ...unboundCandidate(),
        can_create_key: false,
        bindable: false,
        unavailable_reason: "上游分组不可用",
      },
      directGroup,
    );
    expect(
      await screen.findByRole("button", { name: "当前不可探活：上游分组不可用" }),
    ).toBeDisabled();
  },
);
