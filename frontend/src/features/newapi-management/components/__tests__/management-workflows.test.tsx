import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import type { NewAPIRemoteSnapshot } from "@/api";
import { NewAPIGroupBindings, updateBoundGroupRatioSync } from "../group-bindings";
import {
  NewAPIPlatformDetails,
  NewAPIRemoteLoading,
  newAPIViewNeedsRemoteSnapshot,
} from "../newapi-management-page";
import { NewAPIPriceDifferences } from "../model-prices";

describe("New API 页面加载", () => {
  it("主平台不刷新远端数据且渠道读取 New API 分组", () => {
    expect(newAPIViewNeedsRemoteSnapshot("platform")).toBe(false);
    expect(newAPIViewNeedsRemoteSnapshot("channels")).toBe(true);
    expect(newAPIViewNeedsRemoteSnapshot("groups")).toBe(true);
    expect(newAPIViewNeedsRemoteSnapshot("prices")).toBe(true);
    expect(newAPIViewNeedsRemoteSnapshot("differences")).toBe(true);
  });

  it("远端数据读取期间显示具名骨架状态", () => {
    const markup = renderToStaticMarkup(<NewAPIRemoteLoading label="正在加载分组绑定" />);

    expect(markup).toContain('role="status"');
    expect(markup).toContain('aria-label="正在加载分组绑定"');
    expect(markup).toContain("animate-pulse");
  });

  it("主平台明细不重复显示页面顶部摘要", () => {
    const markup = renderToStaticMarkup(
      <NewAPIPlatformDetails
        platform={{
          id: "primary",
          name: "主平台",
          base_url: "https://newapi.example",
          user_id: "1",
          admin_key_configured: true,
          updated_at: "2026-09-02T00:00:00Z",
        }}
      />,
    );

    expect(markup).toContain("平台地址");
    expect(markup).toContain("Admin Key");
    expect(markup).not.toContain("主平台</");
    expect(markup).not.toContain("· User ID");
  });
});

describe("New API 分组绑定", () => {
  it("多项分组时显示中文选择值、统一开关和共享表格容器", () => {
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
    expect(markup).toContain('data-table-panel=""');
    expect(markup).toContain('aria-label="默认 的 Sub2API 分组"');
    expect(markup).toContain('aria-label="VIP 倍率同步"');
    expect(markup).toContain('aria-label="统一倍率同步"');
    expect(markup).toContain(
      'data-slot="select-value" class="flex flex-1 text-left">不绑定</span>',
    );
    expect(markup).not.toContain(
      'data-slot="select-value" class="flex flex-1 text-left">__unbound__',
    );
    expect(markup).toContain(">1<");
    expect(markup).toContain(">2<");
  });

  it("统一倍率同步只更新已绑定分组", () => {
    const enabled = updateBoundGroupRatioSync(
      {
        default: { localGroupId: "", syncRatio: false },
        vip: { localGroupId: "7", syncRatio: false },
      },
      true,
    );

    expect(enabled.default.syncRatio).toBe(false);
    expect(enabled.vip.syncRatio).toBe(true);
    expect(updateBoundGroupRatioSync(enabled, false).vip.syncRatio).toBe(false);
  });

  it("超过十项时只显示当前页并提供翻页控件", () => {
    const groups = Array.from({ length: 11 }, (_, index) => ({
      id: `group-${index + 1}`,
      name: `分组 ${index + 1}`,
      ratio: "1",
    }));
    const markup = renderToStaticMarkup(
      <NewAPIGroupBindings
        groups={groups}
        localGroups={[]}
        bindings={[]}
        pending={false}
        onSave={vi.fn()}
      />,
    );

    expect(markup).toContain('aria-label="转到下一页"');
    expect(markup).toContain("分组 10");
    expect(markup).not.toContain("分组 11");
    expect(markup).toContain("共</span><span");
    expect(markup).toContain(">11</span>");
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
