import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { UpstreamBoundAccount } from "@/api";
import {
  UpstreamBoundAccountSelect,
  upstreamBoundAccountLabel,
} from "../upstream-bound-account-select";

const accounts: UpstreamBoundAccount[] = [
  {
    binding_id: 7,
    account_id: "41",
    account_name: "example-0.1",
    account_exists: true,
    binding_status: "verified",
    local_group: "codex",
    upstream_key_id: "key-1",
    upstream_key_name: "codex-key",
  },
  {
    binding_id: 8,
    account_id: "42",
    account_name: "example-0.2",
    account_exists: true,
    binding_status: "verified",
    local_group: "codex",
    upstream_key_id: "key-2",
    upstream_key_name: "codex-key-2",
  },
];

describe("upstream bound account selection", () => {
  it("shows an explicit unbound state when the group has no account", () => {
    const markup = renderToStaticMarkup(<UpstreamBoundAccountSelect accounts={[]} />);

    expect(markup).toContain("未绑定");
    expect(markup).not.toContain('data-slot="select-trigger"');
  });

  it("shows the selected account and local group on one compact line", () => {
    const markup = renderToStaticMarkup(<UpstreamBoundAccountSelect accounts={accounts} />);

    expect(markup).toContain('data-slot="select-trigger"');
    expect(markup).toContain('aria-label="绑定账号"');
    expect(markup).toContain('data-layout="single-line"');
    expect(markup).toContain("example-0.1");
    expect(markup).toContain("本地分组：codex");
    expect(markup).toContain('aria-label="example-0.1 · 本地分组：codex"');
    expect(markup).not.toContain("data-[size=sm]:min-h-11");
    expect(markup).not.toContain("ID 41");
    expect(markup).not.toContain("key-1");
    expect(markup).not.toContain("已绑定");
    expect(upstreamBoundAccountLabel(accounts[0])).toBe("example-0.1");
  });

  it("shows that a revalidated binding is missing from the management platform", () => {
    const missingAccount = { ...accounts[0], account_name: null, account_exists: false };
    const markup = renderToStaticMarkup(<UpstreamBoundAccountSelect accounts={[missingAccount]} />);

    expect(upstreamBoundAccountLabel(missingAccount)).toBe("管理平台不存在");
    expect(markup).toContain('aria-label="管理平台不存在"');
    expect(markup).not.toContain("本地分组：codex");
  });
});
