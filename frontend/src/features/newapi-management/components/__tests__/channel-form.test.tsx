import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { filterChannelModels } from "../channel-model-dialog";
import { NewAPIChannelConfigurationStep } from "../channel-configuration-step";
import { NewAPIChannelForm } from "../channel-form";

describe("New API 渠道表单", () => {
  it("第一步只选择 Sub2API 分组并创建密钥", () => {
    const markup = renderToStaticMarkup(
      <NewAPIChannelForm
        groups={[{ id: "6", name: "标准", ratio: "1" }]}
        newAPIGroups={[
          { id: "default", name: "默认", ratio: "1" },
          { id: "vip", name: "VIP", ratio: "2" },
        ]}
        sub2APIBaseURL="https://sub2api.example"
        pending={false}
        creatingKey={false}
        fetchingModels={false}
        onCreateKey={vi.fn().mockResolvedValue({ key_id: "key-7", name: "标准", group_id: "6" })}
        onFetchModels={vi.fn().mockResolvedValue(["gpt-5.2"])}
        onSubmit={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    expect(markup).toContain("1. 创建密钥");
    expect(markup).toContain("2. 配置渠道");
    expect(markup).toContain('aria-label="Sub2API 分组"');
    expect(markup).toContain("创建密钥");
    expect(markup).not.toContain("从上游获取");
    expect(markup).not.toContain('aria-label="New API 分组"');
  });

  it("第二步获取模型、选择 New API 分组并添加渠道", () => {
    const markup = renderToStaticMarkup(
      <form>
        <NewAPIChannelConfigurationStep
          channelName="标准"
          sub2APIBaseURL="https://sub2api.example"
          newAPIGroupOptions={[{ value: "default", label: "默认" }]}
          selectedGroups={[]}
          selectedModelCount={0}
          pending={false}
          fetchingModels={false}
          onFetchModels={vi.fn()}
          onGroupsChange={vi.fn()}
        />
      </form>,
    );

    expect(markup).toContain("从上游获取");
    expect(markup).toContain('aria-label="New API 分组"');
    expect(markup).toContain("添加渠道");
    expect(markup).toContain("https://sub2api.example");
  });

  it("模型弹窗搜索不区分大小写", () => {
    expect(filterChannelModels(["GPT-5.2", "claude-sonnet-4"], "gpt")).toEqual(["GPT-5.2"]);
  });
});
