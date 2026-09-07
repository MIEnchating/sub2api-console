import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { ConfigPage } from "@/App";
import { AccountCreationSettingsCard } from "../account-creation-settings-card";

function accountSettingsClient(): QueryClient {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  client.setQueryData(["account-creation-settings"], {
    default: {
      models: ["model-a"],
      concurrency: 10,
      load_factor: null,
      priority: 1,
      pool_mode: false,
      pool_mode_retry_count: 2,
      pool_mode_retry_status_codes: [429],
    },
    groups: [],
    platform_probe_models: {},
  });
  return client;
}

describe("系统设置滚动边界", () => {
  it("账号池模式同步入口位于范围页签上方，并且不占用底部保存区", () => {
    const client = accountSettingsClient();
    render(
      <QueryClientProvider client={client}>
        <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />
      </QueryClientProvider>,
    );
    const sync = screen.getByRole("button", { name: "同步池模式到已有账号" });
    const tabs = screen.getByRole("tablist", { name: "账号设置范围" });
    expect(sync.compareDocumentPosition(tabs) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
    expect(sync.closest('[data-slot="settings-footer"]')).toBeNull();
    expect(sync.closest('[data-slot="settings-scroll"]')).toBeNull();
    expect(
      screen.getByRole("button", { name: "保存全局默认" }).closest('[data-slot="settings-footer"]'),
    ).not.toBeNull();
  });

  it("界面与日志为右侧分配四成宽度，保留天数标签不拆行", () => {
    const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    render(
      <QueryClientProvider client={client}>
        <ConfigPage activeTab="interface" />
      </QueryClientProvider>,
    );
    expect(screen.getByTestId("system-settings-panel")).toHaveClass(
      "xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]",
    );
    expect(screen.getByText("日志保留天数").closest('[data-slot="field-label"]')).toHaveClass(
      "whitespace-nowrap",
    );
  });

  it.each(["connection", "notifications", "interface"] as const)(
    "%s 设置的操作区固定在滚动内容之外",
    (activeTab) => {
      const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
      render(
        <QueryClientProvider client={client}>
          <ConfigPage activeTab={activeTab} />
        </QueryClientProvider>,
      );

      const panel = screen.getByTestId("system-settings-panel");
      expect(panel).toHaveClass("xl:overflow-hidden");
      const footer = panel.querySelector('[data-slot="settings-footer"]');
      expect(footer).toHaveClass("shrink-0");
      expect(footer?.closest('[data-slot="settings-scroll"]')).toBeNull();
    },
  );

  it("切换分组配置后只有分组内容滚动，范围页签和同步操作保持固定", async () => {
    const client = accountSettingsClient();
    render(
      <QueryClientProvider client={client}>
        <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />
      </QueryClientProvider>,
    );
    await userEvent.setup().click(screen.getByRole("tab", { name: "分组独立配置" }));

    expect(
      screen
        .getByRole("tablist", { name: "账号设置范围" })
        .closest('[data-slot="settings-scroll"]'),
    ).toBeNull();
    expect(screen.getByRole("tabpanel", { name: "分组独立配置" })).toHaveClass("overflow-hidden");
    expect(screen.getByText("暂无可配置分组").closest('[data-slot="settings-scroll"]')).toHaveClass(
      "overflow-y-auto",
    );
    expect(
      screen
        .getByRole("button", { name: "同步池模式到已有账号" })
        .closest('[data-slot="settings-scroll"]'),
    ).toBeNull();
  });

  it("全局默认的保存操作位于字段滚动区之外", () => {
    const client = accountSettingsClient();
    render(
      <QueryClientProvider client={client}>
        <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />
      </QueryClientProvider>,
    );
    const save = screen.getByRole("button", { name: "保存全局默认" });
    expect(save.closest('[data-slot="settings-footer"]')).not.toBeNull();
    expect(save.closest('[data-slot="settings-scroll"]')).toBeNull();
    expect(
      screen
        .getByRole("textbox", { name: "全局默认 账号模型" })
        .closest('[data-slot="settings-scroll"]'),
    ).toHaveClass("overflow-y-auto");
  });
});
