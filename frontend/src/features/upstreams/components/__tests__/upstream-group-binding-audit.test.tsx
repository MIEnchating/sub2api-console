import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  summarizeUpstreamGroupBindings,
  UpstreamGroupBindingAuditTable,
  upstreamHasGroupBindingAuditIssue,
} from "../upstream-group-binding-audit";

const items = [
  {
    upstream_id: "up_example",
    host: "api.example.test",
    group_id: "standard",
    group_name: "标准组",
    account_count: 1,
    accounts: [{ id: "41", name: "主账号" }],
    status: "present" as const,
    reason: null,
  },
  {
    upstream_id: "up_example",
    host: "api.example.test",
    group_id: "removed",
    group_name: "已删除组",
    account_count: 1,
    accounts: [{ id: "42", name: null }],
    status: "missing" as const,
    reason: "当前上游目录中已确认不存在",
  },
  {
    upstream_id: "up_example",
    host: "api.example.test",
    group_id: null,
    group_name: "未记录分组",
    account_count: 1,
    accounts: [{ id: "43", name: "待确认账号" }],
    status: "unknown" as const,
    reason: "绑定记录没有稳定的上游分组 ID",
  },
];

describe("UpstreamGroupBindingAuditTable", () => {
  it("shows each bound group status together with its concrete accounts", () => {
    const markup = renderToStaticMarkup(<UpstreamGroupBindingAuditTable items={items} />);

    expect(markup).toContain("存在");
    expect(markup).toContain("缺失");
    expect(markup).toContain("待确认");
    expect(markup).toContain("已删除组");
    expect(markup).toContain("主账号");
    expect(markup).toContain("#41");
    expect(markup).toContain("42");
    expect(markup).toContain("未记录分组 ID");
  });

  it("summarizes present, missing, and unknown groups independently", () => {
    expect(summarizeUpstreamGroupBindings(items)).toEqual({
      present: 1,
      missing: 1,
      unknown: 1,
    });
  });

  it("marks only missing or unconfirmed bindings as issues", () => {
    expect(upstreamHasGroupBindingAuditIssue(items.slice(0, 1))).toBe(false);
    expect(upstreamHasGroupBindingAuditIssue(items.slice(0, 2))).toBe(true);
    expect(upstreamHasGroupBindingAuditIssue([])).toBe(false);
  });
});
