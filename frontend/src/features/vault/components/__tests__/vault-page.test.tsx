import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import type { VaultEntryIndex } from "@/api";
import { VaultEntryTable, VaultPage } from "../vault-page";

describe("VaultEntryTable", () => {
  it("places the credential search toolbar above the table card", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    queryClient.setQueryData(["auth-recovery-config"], {
      vault_entries: [],
    });
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <VaultPage />
      </QueryClientProvider>,
    );
    const toolbarStart = markup.indexOf('data-slot="table-filter-toolbar"');
    const cardStart = markup.indexOf('data-slot="card"');

    expect(toolbarStart).toBeGreaterThan(-1);
    expect(toolbarStart).toBeLessThan(cardStart);
    expect(markup).toContain('aria-label="搜索凭据"');
    expect(markup).toContain('data-table-panel=""');
  });

  it("shows only the redacted password entry index and exposes edit and delete actions", () => {
    const entry: VaultEntryIndex = {
      entry: "operator",
      hosts: ["api.example.test", "backup.example.test"],
      has_username: true,
      has_password: true,
      username_is_email: true,
      header_names: ["X-Custom-Header"],
    };

    const markup = renderToStaticMarkup(
      <VaultEntryTable entries={[entry]} onEdit={() => {}} onDelete={() => {}} />,
    );

    expect(markup).toContain("operator");
    expect(markup).toContain("api.example.test");
    expect(markup).toContain("backup.example.test");
    expect(markup).toContain("api.example.test、backup.example.test");
    expect(markup).toContain("凭据完整");
    expect(markup).toContain("X-Custom-Header");
    expect(markup).not.toContain("Base URL");
    expect(markup).toContain('aria-label="编辑凭据"');
    expect(markup).toContain('aria-label="删除凭据"');
    expect(markup).toContain("text-destructive");
    expect(markup).not.toContain("title=");
    expect(markup).not.toContain("username-value");
    expect(markup).not.toContain("password-value");
  });

  it("describes missing credentials without inventing sensitive values", () => {
    const entry: VaultEntryIndex = {
      entry: "fallback",
      hosts: [],
      has_username: false,
      has_password: false,
      username_is_email: false,
      header_names: [],
    };

    const markup = renderToStaticMarkup(
      <VaultEntryTable entries={[entry]} onEdit={() => {}} onDelete={() => {}} />,
    );

    expect(markup).toContain("全部 Host");
    expect(markup).toContain("缺少用户名、密码");
    expect(markup).toContain("未设置");
  });
});
