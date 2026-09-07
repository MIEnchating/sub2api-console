import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import type { UpstreamConfiguration } from "@/api";
import { dialogBodyLayout, dialogContentClass } from "@/components/ui/dialog";
import {
  UpstreamAccounts,
  UpstreamEditDialog,
  currentUpstreamGroupStatus,
  upstreamEditConnectionLabels,
  upstreamEditDialogLayout,
  upstreamEditPresentation,
  upstreamEditSectionOrder,
} from "../upstream-edit-dialog";

describe("upstream edit dialog", () => {
  it("uses an icon tooltip for the custom Headers helper", async () => {
    const configuration: UpstreamConfiguration = {
      upstream_id: "up_example",
      host: "api.example.test",
      name: "Example",
      base_url: "https://api.example.test",
      account_base_url: "https://api.example.test/v1",
      upstream_type: "sub2api",
      auth_mode: "sub2api_user_token",
      recharge_rate: "1",
      raw_balance: null,
      balance: null,
      has_access_token: true,
      has_refresh_token: false,
      has_admin_key: false,
      has_user_id: false,
      headers: { "X-Client": "console" },
      header_names: ["X-Client"],
      cookie_names: [],
      groups: [],
    };
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    queryClient.setQueryData(["upstream-configuration", configuration.host], configuration);
    queryClient.setQueryData(["auth-recovery-config"], { vault_entries: [] });

    render(
      <QueryClientProvider client={queryClient}>
        <UpstreamEditDialog
          host={configuration.host}
          onOpenChange={() => undefined}
          onSaved={() => undefined}
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("button", { name: "自定义 Headers说明" })).toBeVisible();
    expect(screen.queryByText("已配置：X-Client")).not.toBeInTheDocument();
  });

  it("edits upstream Host and account Base URL in the upstream dialog", () => {
    expect(upstreamEditConnectionLabels).toEqual({
      upstreamHost: "上游 Host",
      accountBaseURL: "账号 Base URL",
    });
  });

  it("places recharge conversion directly above current upstream accounts", () => {
    expect(upstreamEditSectionOrder).toEqual(["connection", "recharge", "accounts"]);
  });

  it("shows every binding under the current upstream and marks duplicate accounts", () => {
    const configuration: UpstreamConfiguration = {
      upstream_id: "up_example",
      host: "api.example.test",
      name: "Example",
      base_url: "https://api.example.test",
      account_base_url: "https://account-api.example.test/v1",
      upstream_type: "newapi",
      auth_mode: "newapi_admin_key",
      recharge_rate: "5",
      raw_balance: "25",
      balance: "5",
      has_access_token: false,
      has_refresh_token: false,
      has_admin_key: true,
      has_user_id: true,
      headers: {},
      header_names: [],
      cookie_names: [],
      groups: [
        {
          upstream_id: "up_example",
          host: "api.example.test",
          group_id: "codex",
          name: "codex",
          description: "Codex pool",
          platform: "openai",
          status: "active",
          raw_rate: "0.2",
          effective_rate: "0.04",
          recharge_rate: "5",
          bound: false,
          bound_accounts: [
            {
              binding_id: 1,
              account_id: "41",
              account_name: "Codex 主账号",
              account_exists: true,
              binding_status: "active",
              local_group: "codex-local",
              upstream_key_id: "key-41",
              upstream_key_name: "主 Key",
            },
            {
              binding_id: 2,
              account_id: "42",
              account_name: "Codex 备用账号",
              account_exists: false,
              binding_status: "missing",
              local_group: "codex-local",
              upstream_key_id: "key-42",
              upstream_key_name: "备用 Key",
            },
          ],
          key_present: true,
          bindable: true,
          unavailable_reason: null,
        },
        {
          upstream_id: "up_example",
          host: "api.example.test",
          group_id: "claude",
          name: "claude",
          description: "Claude pool",
          platform: "anthropic",
          status: "active",
          raw_rate: "0.3",
          effective_rate: "0.06",
          recharge_rate: "5",
          bound: true,
          bound_accounts: [
            {
              binding_id: 3,
              account_id: "41",
              account_name: "Codex 主账号",
              account_exists: true,
              binding_status: "active",
              local_group: "claude-local",
              upstream_key_id: "key-41",
              upstream_key_name: "主 Key",
            },
            {
              binding_id: 4,
              account_id: "43",
              account_name: "Claude 账号",
              account_exists: true,
              binding_status: "active",
              local_group: "claude-local",
              upstream_key_id: "key-43",
              upstream_key_name: "Claude Key",
            },
          ],
          key_present: true,
          bindable: false,
          unavailable_reason: null,
        },
      ],
    };
    const presentation = upstreamEditPresentation(configuration);
    const accounts = renderToStaticMarkup(<UpstreamAccounts groups={configuration.groups} />);

    expect(presentation.adminKeyState).toBe("已配置，留空则不修改");
    expect(presentation.rawBalance).toBe("25");
    expect(presentation.mappedBalance).toBe("5");
    expect(presentation).not.toHaveProperty("groups");
    expect(JSON.stringify(presentation)).not.toContain("secret");
    expect(accounts).toContain("4 条绑定");
    expect(accounts).toContain("3 个账号");
    expect(accounts).toContain(">账号</span>");
    expect(accounts).toContain(">上游分组</span>");
    expect(accounts).toContain(">状态</span>");
    expect(accounts).toContain(">操作</span>");
    expect(accounts).toContain("Codex 主账号");
    expect(accounts.match(/稳定账号 ID 41/g)).toHaveLength(2);
    expect(accounts.match(/重复绑定/g)).toHaveLength(2);
    expect(accounts).toContain(">codex</span>");
    expect(accounts).toContain(">claude</span>");
    expect(accounts.match(/存在 · 启用/g)).toHaveLength(4);
    expect(accounts).toContain("Codex 备用账号");
    expect(accounts).toContain("稳定账号 ID 42");
    expect(accounts).toContain("Claude 账号");
    expect(accounts).toContain("稳定账号 ID 43");
    expect(accounts).toContain("账号不存在");
  });

  it("区分上游分组存在时的启禁用状态和已不存在状态", () => {
    expect(currentUpstreamGroupStatus("active")).toEqual({
      label: "存在 · 启用",
      badgeVariant: "secondary",
    });
    expect(currentUpstreamGroupStatus("disabled")).toEqual({
      label: "存在 · 禁用",
      badgeVariant: "warning",
    });
    expect(currentUpstreamGroupStatus("missing")).toEqual({
      label: "已不存在",
      badgeVariant: "destructive",
    });
    expect(currentUpstreamGroupStatus("suspected")).toEqual({
      label: "待确认",
      badgeVariant: "warning",
    });
  });

  it("shows an empty state when the current upstream has no accounts", () => {
    const accounts = renderToStaticMarkup(
      <UpstreamAccounts
        groups={[
          {
            upstream_id: "up_example",
            host: "api.example.test",
            group_id: "claude",
            name: "claude",
            description: null,
            platform: "anthropic",
            status: "active",
            raw_rate: "0.3",
            effective_rate: "0.06",
            recharge_rate: "5",
            bound: false,
            bound_accounts: [],
            key_present: false,
            bindable: true,
            unavailable_reason: null,
          },
        ]}
      />,
    );

    expect(accounts).toContain("0 条绑定");
    expect(accounts).toContain("0 个账号");
    expect(accounts).toContain("当前上游暂无绑定账号");
  });

  it("keeps vertical scrolling without exposing a horizontal scroll area", () => {
    const content = dialogContentClass("wide", "tall", upstreamEditDialogLayout.content);
    expect(content).toContain("overflow-hidden");
    expect(content).toContain("w-[min(64rem,calc(100vw-2rem))]");
    expect(content).toContain("max-h-[min(44rem,calc(100svh-2rem))]");
    expect(dialogBodyLayout).toContain("min-w-0");
    expect(dialogBodyLayout).toContain("overflow-y-auto");
    expect(upstreamEditDialogLayout.scrollArea).toContain("overflow-x-clip");
    expect(upstreamEditDialogLayout.form).toContain("min-w-0");
    expect(upstreamEditDialogLayout.form).toContain("max-w-full");
  });
});
