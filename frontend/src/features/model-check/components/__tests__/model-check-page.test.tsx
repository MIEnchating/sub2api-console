import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { modelCheckDialogLayout, ModelCheckPage } from "../model-check-page";

describe("模型检测页面", () => {
  it("运行和结果阶段使用稳定的大尺寸弹窗", () => {
    expect(
      modelCheckDialogLayout({
        id: "model-check-running",
        skill: "sub2api-model-check",
        operation: "account-model-behavior-check",
        status: "running",
        progress: 14,
        message: "已完成 2/20 个账号模型组合",
        result: {},
        created_at: "2026-08-31T00:00:00Z",
        updated_at: "2026-08-31T00:00:01Z",
      }),
    ).toEqual({ width: "table", height: "tall", resultsReady: false });

    expect(
      modelCheckDialogLayout({
        id: "model-check-succeeded",
        skill: "sub2api-model-check",
        operation: "account-model-behavior-check",
        status: "succeeded",
        progress: 100,
        message: "账号模型检测完成",
        result: { tests: [] },
        created_at: "2026-08-31T00:00:00Z",
        updated_at: "2026-08-31T00:00:02Z",
      }),
    ).toEqual({ width: "table", height: "tall", resultsReady: true });
  });

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
    queryClient.setQueryData(
      ["model-check-account-statuses"],
      [
        {
          account_id: "41",
          status: "inconclusive",
          checked_at: "2026-08-31T02:15:00Z",
          task_id: "model-check-previous",
        },
      ],
    );

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
    expect(markup).not.toContain("健康");
    expect(markup).toContain("无法判定");
    expect(markup).toContain("上次检测");
  });
});
