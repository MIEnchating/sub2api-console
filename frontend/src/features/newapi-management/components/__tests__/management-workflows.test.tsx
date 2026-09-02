import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import type { NewAPIRemoteSnapshot } from "@/api";
import { NewAPIGroupBindings } from "../group-bindings";
import { NewAPIPriceDifferences } from "../model-prices";

describe("New API 分组绑定", () => {
  it("多项分组时显示稳定分组选择和独立倍率同步开关", () => {
    const markup = renderToStaticMarkup(
      <NewAPIGroupBindings
        groups={[
          { id: "default", name: "默认", ratio: "1" },
          { id: "vip", name: "VIP", ratio: "2" },
        ]}
        localGroups={[
          { id: "6", name: "标准", ratio: "0.5" },
          { id: "7", name: "高速", ratio: "1.2" },
        ]}
        bindings={[
          {
            platform_id: "platform-1",
            newapi_group_id: "vip",
            newapi_group_name: "VIP",
            sub2api_group_id: "7",
            sync_ratio: true,
          },
        ]}
        pending={false}
        onSave={vi.fn()}
      />,
    );

    expect(markup).toContain("分组绑定");
    expect(markup).toContain('aria-label="默认 的 Sub2API 分组"');
    expect(markup).toContain('aria-label="VIP 倍率同步"');
    expect(markup).toContain(">1<");
    expect(markup).toContain(">2<");
  });

  it("空数据时保留明确空状态并禁用保存", () => {
    const markup = renderToStaticMarkup(
      <NewAPIGroupBindings
        groups={[]}
        localGroups={[]}
        bindings={[]}
        pending={false}
        onSave={vi.fn()}
      />,
    );

    expect(markup).toContain("尚未读取到 New API 分组");
    expect(markup).toContain("disabled");
  });
});

describe("New API 价格差异", () => {
  it("显示缺失、仅配置和倍率不同三种结果", () => {
    const snapshot: NewAPIRemoteSnapshot = {
      groups: [],
      models: [],
      references: [{ model: "gpt-5", input_ratio: "0.625", completion_ratio: "8" }],
      differences: [
        {
          model: "gpt-5",
          kind: "ratio_mismatch",
          configured: { model: "gpt-5", input_ratio: "0.5", completion_ratio: "8" },
          reference: { model: "gpt-5", input_ratio: "0.625", completion_ratio: "8" },
        },
        {
          model: "claude-sonnet-4",
          kind: "missing_in_newapi",
          configured: null,
          reference: {
            model: "claude-sonnet-4",
            input_ratio: "1.5",
            completion_ratio: "5",
          },
        },
      ],
      fetched_at: "2026-09-02T00:00:00Z",
    };

    const markup = renderToStaticMarkup(<NewAPIPriceDifferences snapshot={snapshot} />);

    expect(markup).toContain("需要处理");
    expect(markup).toContain("倍率不同");
    expect(markup).toContain("New API 缺失");
    expect(markup).toContain("0.5 / 8");
    expect(markup).toContain("1.5 / 5");
  });
});
