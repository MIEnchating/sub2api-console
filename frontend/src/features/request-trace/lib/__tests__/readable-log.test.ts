import { describe, expect, it } from "vitest";
import { readableSystemLog, readableUsage } from "../readable-log";

describe("日志展示", () => {
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

  it("formats available token usage for request result details", () => {
    expect(
      readableUsage({
        usage_available: true,
        input_tokens: "1000",
        output_tokens: "25",
        cache_read_tokens: "500",
        cache_write_tokens: "0",
      }),
    ).toEqual({ input: "1,000", output: "25", cacheRead: "500", cacheWrite: "0" });
  });

  it("does not render an empty usage section when usage is unavailable", () => {
    expect(
      readableUsage({
        usage_available: false,
        input_tokens: null,
        output_tokens: null,
        cache_read_tokens: null,
        cache_write_tokens: null,
      }),
    ).toBeNull();
  });
});
