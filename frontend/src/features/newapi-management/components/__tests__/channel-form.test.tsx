import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { filterChannelModels } from "../channel-model-dialog";
import { NewAPIChannelConfigurationStep } from "../channel-configuration-step";
import { NewAPIChannelForm } from "../channel-form";

describe("New API 渠道表单", () => {
  it("第一步选择普通账号来源、Sub2API 分组并创建密钥", () => {
    const markup = renderToStaticMarkup(
      <NewAPIChannelForm
        groups={[{ id: "6", name: "标准", ratio: "1" }]}
        newAPIGroups={[
          { id: "default", name: "默认", ratio: "1" },
          { id: "vip", name: "VIP", ratio: "2" },
        ]}
        sub2APIBaseURL="https://sub2api.example"
        vaultEntries={[
          {
            entry: "运营账号",
            hosts: ["sub2api.example"],
            has_username: true,
            has_password: true,
            username_is_email: true,
            header_names: [],
          },
        ]}
        pending={false}
        creatingKey={false}
        fetchingModels={false}
        onCreateKey={vi.fn().mockResolvedValue({
          key_id: "key-7",
          name: "标准",
          group_id: "6",
          endpoints: [{ name: "API 端点", base_url: "https://api.example", default: true }],
        })}
        onFetchModels={vi.fn().mockResolvedValue(["gpt-5.2"])}
        onSubmit={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    expect(markup).toContain("1. 创建密钥");
    expect(markup).toContain("2. 配置渠道");
    expect(markup).toContain('aria-label="账号来源"');
    expect(markup).toContain("密码箱账号");
    expect(markup).toContain("自定义账号密码");
    expect(markup).toContain('aria-label="密码箱账号"');
    expect(markup).toContain("运营账号");
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
          apiEndpoints={[
            { name: "API 端点", base_url: "https://api.example", default: true },
            { name: "Docker 内网", base_url: "http://sub2api:8080", default: false },
          ]}
          baseURL="https://api.example"
          customBaseURL={false}
          newAPIGroupOptions={[{ value: "default", label: "默认" }]}
          selectedGroups={[]}
          selectedModelCount={0}
          pending={false}
          fetchingModels={false}
          onFetchModels={vi.fn()}
          onBaseURLModeChange={vi.fn()}
          onBaseURLChange={vi.fn()}
          onGroupsChange={vi.fn()}
        />
      </form>,
    );

    expect(markup).toContain("从上游获取");
    expect(markup).toContain('aria-label="New API 分组"');
    expect(markup).toContain("添加渠道");
    expect(markup).toContain('aria-label="API 地址来源"');
    expect(markup).toContain("https://api.example");
  });

  it("第二步选择自定义来源时显示可编辑 API 地址", () => {
    const markup = renderToStaticMarkup(
      <form>
        <NewAPIChannelConfigurationStep
          channelName="标准"
          sub2APIBaseURL="https://sub2api.example"
          apiEndpoints={[{ name: "API 端点", base_url: "https://api.example", default: true }]}
          baseURL="https://custom.example"
          customBaseURL
          newAPIGroupOptions={[{ value: "default", label: "默认" }]}
          selectedGroups={[]}
          selectedModelCount={0}
          pending={false}
          fetchingModels={false}
          onFetchModels={vi.fn()}
          onBaseURLModeChange={vi.fn()}
          onBaseURLChange={vi.fn()}
          onGroupsChange={vi.fn()}
        />
      </form>,
    );

    expect(markup).toContain('aria-label="自定义 API 地址"');
    expect(markup).toContain('value="https://custom.example"');
  });

  it("模型弹窗搜索不区分大小写", () => {
    expect(filterChannelModels(["GPT-5.2", "claude-sonnet-4"], "gpt")).toEqual(["GPT-5.2"]);
  });
});
