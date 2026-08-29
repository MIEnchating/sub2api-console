import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import {
  nextSystemLogSubmission,
  readableSystemLog,
  SystemLogSearchPanel,
} from "../system-log-search-panel";

describe("SystemLogSearchPanel", () => {
  it("shows the same searchable fields as the Sub2API system log page", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <SystemLogSearchPanel />
      </QueryClientProvider>,
    );

    for (const label of [
      "时间范围",
      "开始时间（可选）",
      "结束时间（可选）",
      "级别",
      "Host",
      "request_id",
      "client_request_id",
      "KEY ID",
      "account_id",
      "平台",
      "模型",
      "关键词",
    ]) {
      expect(markup).toContain(label);
    }
    expect(markup).toContain("查询 Sub2API 系统日志");
    expect(markup).toContain('type="datetime-local"');
    expect(markup).not.toContain("组件");
    expect(markup).not.toContain("user_id");
  });

  it("turns the compact HTTP log message into readable request fields", () => {
    const log = readableSystemLog({
      id: 1,
      request_id: "4031aae9-e191-4955-b38d-5eec098ef571",
      account_id: null,
      account_name: "星筱 AI-1",
      group_name: null,
      is_error: false,
      error_reason: null,
      first_token_ms: null,
      duration_ms: null,
      summary:
        "http request completed status=200 latency_ms=20093 method=POST path=/v1/responses ip=172.18.0.7 proto=HTTP/1.1 req=4031aae9-e191-4955-b38d-5eec098ef571 client_req=15139114-18df-499e-81b2-6ccb7e515b11 acc=22 platform=openai model=gpt-5.6-sol",
      observed_at: "2026-08-29T00:00:00Z",
      source: "system-log",
      payload: {},
    });

    expect(log).toMatchObject({
      title: "HTTP 请求完成",
      status: "200",
      duration: "20093",
      accountId: "22",
      accountName: "星筱 AI-1",
      account: "星筱 AI-1（ID：22）",
      method: "POST",
      path: "/v1/responses",
      platform: "openai",
      model: "gpt-5.6-sol",
      clientRequestId: "15139114-18df-499e-81b2-6ccb7e515b11",
    });
  });

  it("uses a concise title for account test failures", () => {
    const log = readableSystemLog({
      id: 2,
      request_id: "",
      account_id: "298",
      account_name: "星筱 AI-2",
      group_name: null,
      is_error: true,
      error_reason: "API returned 403",
      first_token_ms: null,
      duration_ms: null,
      summary: "Account test error: API returned 403: user quota exceeded",
      observed_at: "2026-08-29T14:22:14Z",
      source: "system-log",
      payload: {},
    });

    expect(log.title).toBe("账号测试失败");
    expect(log.account).toBe("星筱 AI-2（ID：298）");
  });

  it("creates a new execution when the same search is submitted again", () => {
    const query = {
      timeRange: "1h" as const,
      startTime: "",
      endTime: "",
      host: "",
      level: "",
      requestId: "req-1",
      clientRequestId: "",
      apiKeyId: "",
      accountId: "",
      platform: "",
      model: "",
      keyword: "",
      page: 1,
      pageSize: 20,
    };

    const first = nextSystemLogSubmission(null, query);
    const second = nextSystemLogSubmission(first, query);

    expect(first.query).toEqual(second.query);
    expect(second.execution).toBe(first.execution + 1);
  });
});
