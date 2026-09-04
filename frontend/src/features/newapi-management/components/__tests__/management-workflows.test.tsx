import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import type { NewAPIRemoteSnapshot } from "@/api";
import { NewAPIGroupBindings, updateBoundGroupRatioSync } from "../group-bindings";
import {
  NewAPIHeadingAction,
  NewAPIPlatformDetails,
  NewAPIRemoteLoading,
  newAPIRemoteSnapshotQueryKey,
  newAPIViewNeedsRemoteSnapshot,
} from "../newapi-management-page";
import { NewAPIModelPrices, NewAPIPriceDifferences } from "../model-prices";

describe("New API 页面加载", () => {
  it("主平台不刷新远端数据且渠道读取 New API 分组", () => {
    expect(newAPIViewNeedsRemoteSnapshot("platform")).toBe(false);
    expect(newAPIViewNeedsRemoteSnapshot("channels")).toBe(true);
    expect(newAPIViewNeedsRemoteSnapshot("groups")).toBe(true);
    expect(newAPIViewNeedsRemoteSnapshot("prices")).toBe(true);
    expect(newAPIViewNeedsRemoteSnapshot("differences")).toBe(true);
  });

  it("模型价格和价格差异复用同一平台快照查询", () => {
    expect(newAPIViewNeedsRemoteSnapshot("prices")).toBe(true);
    expect(newAPIViewNeedsRemoteSnapshot("differences")).toBe(true);
    expect(newAPIRemoteSnapshotQueryKey("primary")).toEqual(["newapi-remote-snapshot", "primary"]);
  });

  it("远端数据读取期间显示具名骨架状态", () => {
    const markup = renderToStaticMarkup(<NewAPIRemoteLoading label="正在加载分组绑定" />);

    expect(markup).toContain('role="status"');
    expect(markup).toContain('aria-label="正在加载分组绑定"');
    expect(markup).toContain("animate-pulse");
  });

  it("已配置时页面顶部只在远端页面显示刷新操作", () => {
    const remoteMarkup = renderToStaticMarkup(
      <NewAPIHeadingAction
        hasPlatform
        needsRemoteSnapshot
        refreshPending={false}
        onRefresh={vi.fn()}
        onConfigure={vi.fn()}
      />,
    );
    const configurationMarkup = renderToStaticMarkup(
      <NewAPIHeadingAction
        hasPlatform
        needsRemoteSnapshot={false}
        refreshPending={false}
        onRefresh={vi.fn()}
        onConfigure={vi.fn()}
      />,
    );

    expect(remoteMarkup).toContain('aria-label="刷新 New API 数据"');
    expect(remoteMarkup).not.toContain("编辑");
    expect(configurationMarkup).toBe("");
  });

  it("配置明细在内容区提供编辑和删除入口", () => {
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
        onEdit={vi.fn()}
        onDelete={vi.fn()}
      />,
    );

    expect(markup).toContain("平台配置");
    expect(markup).toContain("平台地址");
    expect(markup).toContain("Admin Key");
    expect(markup).toContain("编辑配置");
    expect(markup).toContain('aria-label="删除 New API 配置"');
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

  it("超过二十项时只显示当前页并提供翻页控件", () => {
    const groups = Array.from({ length: 21 }, (_, index) => ({
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
    expect(markup).toContain("分组 20");
    expect(markup).not.toContain("分组 21");
    expect(markup).toContain("共</span><span");
    expect(markup).toContain(">21</span>");
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
  it("按 New API 页面显示价格分类但不在页签中显示数量", () => {
    const models = Array.from({ length: 33 }, (_, index) => ({
      model: `configured-${index + 1}`,
      input_ratio: "1",
      completion_ratio: "2",
      billing_mode: "per-token",
    }));
    const unsetModels = Array.from({ length: 3 }, (_, index) => ({
      model: `unset-${index + 1}`,
      input_ratio: "",
      completion_ratio: "",
      billing_mode: "per-token",
    }));
    const toolPrices = Array.from({ length: 9 }, (_, index) => ({
      tool: `tool-${index + 1}`,
      price: String(index + 1),
    }));

    const markup = renderToStaticMarkup(
      <NewAPIModelPrices
        models={models}
        unsetModels={unsetModels}
        toolPrices={toolPrices}
        onCompareManagementPrices={vi.fn()}
        onViewRawPricingSource={vi.fn()}
      />,
    );

    expect(markup).toContain(">模型价格</button>");
    expect(markup).toContain(">未设置模型价格</button>");
    expect(markup).toContain(">工具价格</button>");
    expect(markup).toContain(">远程模型价格</button>");
    expect(markup).not.toContain("模型价格（33）");
    expect(markup).not.toContain("未设置模型价格（3）");
    expect(markup).not.toContain("工具价格（9）");
    expect(markup).toContain("按 Token");
    expect(markup).toContain("比较模型价格");
    expect(markup).toContain("查看原始价卡");
    expect(markup).toContain(">状态</th>");
    expect(markup).toContain("未比较");
    expect(markup).not.toContain("查询模型价格");
  });

  it("以只读价格文本显示 New API 模型并按二十项分页", () => {
    const models = Array.from({ length: 21 }, (_, index) => ({
      model: `model-${String(index + 1).padStart(2, "0")}`,
      input_ratio: "1",
      completion_ratio: "2",
    }));
    const markup = renderToStaticMarkup(<NewAPIModelPrices models={models} />);

    expect(markup).toContain("输入价格");
    expect(markup).toContain("输出价格");
    expect(markup).toContain("text-right");
    expect(markup).toContain("align-top");
    expect(markup).not.toContain('inputmode="decimal"');
    expect(markup).not.toContain("保存修改");
    expect(markup).toContain('aria-label="转到下一页"');
    expect(markup).toContain(">model-20<");
    expect(markup).not.toContain(">model-21<");
  });

  it("只显示 New API 当前配置模型", () => {
    const configured = Array.from({ length: 13 }, (_, index) => ({
      model: `catalog-model-${String(index + 1).padStart(2, "0")}`,
      input_ratio: "1",
      completion_ratio: "2",
    }));
    const markup = renderToStaticMarkup(<NewAPIModelPrices models={configured} />);

    expect(markup).toContain("catalog-model-01");
    expect(markup).not.toContain("catalog-model-20");
  });

  it("只显示本平台模型价格且不显示差异比较状态", () => {
    const snapshot: NewAPIRemoteSnapshot = {
      groups: [],
      models: [
        { model: "gpt-5", input_ratio: "0.5", completion_ratio: "8" },
        { model: "claude-sonnet-4", input_ratio: "1.5", completion_ratio: "5" },
      ],
      unset_models: [],
      tool_prices: [],
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
          kind: "missing_in_model_plaza",
          configured: {
            model: "claude-sonnet-4",
            input_ratio: "1.5",
            completion_ratio: "5",
          },
          reference: null,
        },
      ],
      fetched_at: "2026-09-02T00:00:00Z",
    };

    const markup = renderToStaticMarkup(<NewAPIPriceDifferences snapshot={snapshot} />);

    expect(markup).not.toContain("需要处理");
    expect(markup).not.toContain("倍率不同");
    expect(markup).toContain("输入价格");
    expect(markup).toContain("输出价格");
    expect(markup).toContain("按 Token");
  });
});
