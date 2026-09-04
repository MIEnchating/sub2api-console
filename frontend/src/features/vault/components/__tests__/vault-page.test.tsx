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
    expect(markup).toContain('aria-label="状态筛选"');
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

  it("condenses long Host lists instead of expanding the table row indefinitely", () => {
    const entry: VaultEntryIndex = {
      entry: "many-hosts",
      hosts: Array.from({ length: 10 }, (_, index) => `api-${index}.example.test`),
      has_username: true,
      has_password: true,
      username_is_email: true,
      header_names: [],
    };

    const markup = renderToStaticMarkup(
      <VaultEntryTable entries={[entry]} onEdit={() => {}} onDelete={() => {}} />,
    );

    expect(markup).toContain("+3 个");
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

  it("warns when multiple vault entries claim the same Host", () => {
    const entry: VaultEntryIndex = {
      entry: "duplicate-host",
      hosts: ["api.example.test"],
      has_username: true,
      has_password: true,
      username_is_email: true,
      header_names: [],
    };

    const markup = renderToStaticMarkup(
      <VaultEntryTable
        entries={[entry]}
        conflictEntries={new Set([entry.entry])}
        onEdit={() => {}}
        onDelete={() => {}}
      />,
    );

    expect(markup).toContain("存在 Host 匹配冲突");
  });

  it("warns when a wildcard vault entry overlaps an explicit Host", () => {
    const wildcard: VaultEntryIndex = {
      entry: "all-hosts",
      hosts: [],
      has_username: true,
      has_password: true,
      username_is_email: true,
      header_names: [],
    };
    const explicit: VaultEntryIndex = {
      ...wildcard,
      entry: "specific",
      hosts: ["api.example.test"],
    };
    const markup = renderToStaticMarkup(
      <VaultEntryTable
        entries={[wildcard, explicit]}
        conflictEntries={new Set([wildcard.entry, explicit.entry])}
        onEdit={() => {}}
        onDelete={() => {}}
      />,
    );

    expect(markup.match(/存在 Host 匹配冲突/g)?.length).toBe(2);
  });
});
