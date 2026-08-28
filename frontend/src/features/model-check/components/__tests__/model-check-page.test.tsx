import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ModelCheckPage } from "../model-check-page";

describe("模型检测页面", () => {
  it("使用账号与模型矩阵选择界面且不展示接口凭据字段", () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(
      ["accounts"],
      [
        {
          id: "41",
          name: "主账号",
          groups: ["default"],
          platform: "claude",
          account_type: "oauth",
          health: "healthy",
          health_status: "健康",
          paused: false,
          schedulable: true,
        },
      ],
    );
    queryClient.setQueryData(["model-check-capabilities"], {
      claude_standards: ["claude-opus-5"],
      sol_models: ["gpt-5.6-sol", "gpt-5.6-luna", "gpt-5.6-terra"],
    });

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <ModelCheckPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain("模型检测");
    expect(markup).toContain("全选账号");
    expect(markup).toContain("刷新模型");
    expect(markup).toContain("主账号");
    expect(markup).toContain("请先选择账号");
    expect(markup).toContain("开始检测");
    expect(markup).not.toContain("接口地址");
    expect(markup).not.toContain("API Key");
  });
});
