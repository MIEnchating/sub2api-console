import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type AccountCreationPolicy } from "@/api";
import { AccountCreationPolicyForm } from "../account-creation-policy-form";
import { ModelSyncSettingsCard } from "../model-sync-settings-card";
import { PlatformProbeModelsForm } from "../platform-probe-models-form";

const policy: AccountCreationPolicy = {
  models: ["model-a"],
  concurrency: 10,
  load_factor: "1",
  priority: 1,
  pool_mode: true,
  pool_mode_retry_count: 3,
  pool_mode_retry_status_codes: [429],
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("开户策略保存期间锁定字段，保存结束后恢复编辑", () => {
  const form = (pending: boolean) => (
    <AccountCreationPolicyForm
      formId="policy"
      scopeLabel="全局默认"
      policy={policy}
      pending={pending}
      submitLabel="保存全局默认"
      onSubmit={vi.fn()}
    />
  );
  const view = render(form(true));
  for (const field of [...screen.getAllByRole("textbox"), ...screen.getAllByRole("spinbutton")]) {
    expect(field).toBeDisabled();
  }
  expect(screen.getByRole("switch", { name: /池模式/ })).toHaveAttribute("aria-disabled", "true");
  view.rerender(form(false));
  expect(screen.getByRole("textbox", { name: "全局默认 账号模型" })).toBeEnabled();
});

it("平台探活模型保存期间锁定字段，保存结束后恢复编辑", () => {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  const form = (pending: boolean) => (
    <QueryClientProvider client={client}>
      <PlatformProbeModelsForm
        models={{ openai: "probe-a" }}
        pending={pending}
        onSubmit={vi.fn()}
      />
    </QueryClientProvider>
  );
  const view = render(form(true));
  for (const field of screen.getAllByRole("textbox")) expect(field).toBeDisabled();
  view.rerender(form(false));
  expect(screen.getByRole("textbox", { name: "OpenAI 默认探活模型" })).toBeEnabled();
  view.unmount();
  client.clear();
});

it("屏蔽规则保存等待期间锁定输入，失败后保留草稿并恢复编辑", async () => {
  let fail!: (reason: Error) => void;
  vi.spyOn(api, "updateAccountModelSyncSettings").mockReturnValue(
    new Promise((_resolve, reject) => {
      fail = reject;
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  client.setQueryData(["account-model-sync-settings"], { blocked_patterns: ["model-a"] });
  render(
    <QueryClientProvider client={client}>
      <ModelSyncSettingsCard />
    </QueryClientProvider>,
  );
  const field = screen.getByRole("textbox", { name: "屏蔽规则" });
  fireEvent.change(field, { target: { value: "model-b" } });
  fireEvent.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存中…" })).toBeDisabled());
  expect(field).toBeDisabled();
  await act(async () => fail(new Error("保存失败")));
  await waitFor(() => expect(field).toBeEnabled());
  expect(field).toHaveValue("model-b");
  cleanup();
  client.clear();
});
