import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import {
  nextSystemLogSubmission,
  readableSystemLog,
  SystemLogSearchPanel,
} from "../system-log-search-panel";

describe("SystemLogSearchPanel", () => {
  it("shows only request_id as a searchable field", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <SystemLogSearchPanel />
      </QueryClientProvider>,
    );

    expect(markup).toContain("request_id");
    expect(markup).toContain("输入完整 request_id");
    expect(markup).toContain('data-slot="table-filter-toolbar"');
    expect(markup).toContain("flex min-w-0 items-center gap-3");
    expect(markup).toContain("shrink-0 whitespace-nowrap");
    expect(markup).toContain("min-w-0 flex-1");
    expect(markup).toContain("min-w-0 basis-72 flex-1");
    expect(markup).not.toContain("查询 Sub2API 系统日志");
    expect(markup).not.toContain("快速定位对应的请求日志和执行结果");
    for (const label of [
      "时间范围",
      "开始时间（可选）",
      "结束时间（可选）",
      "级别",
      "Host",
      "client_request_id",
      "KEY ID",
      "account_id",
      "平台",
      "模型",
      "关键词",
    ]) {
      expect(markup).not.toContain(label);
    }
  });

  it("fills the remaining area with a placeholder before the first search", () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <SystemLogSearchPanel />
      </QueryClientProvider>,
    );

    expect(markup).toContain('data-slot="request-trace-placeholder"');
    expect(markup).toContain("min-h-0 flex-1");
    expect(markup).toContain("items-center justify-center");
    expect(markup).toContain("输入 request_id 开始查询");
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
      requestId: "req-1",
      page: 1,
      pageSize: 20,
    };

    const first = nextSystemLogSubmission(null, query);
    const second = nextSystemLogSubmission(first, query);

    expect(first.query).toEqual(second.query);
    expect(second.execution).toBe(first.execution + 1);
  });
});
