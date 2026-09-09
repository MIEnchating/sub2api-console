import { describe, expect, it } from "vitest";

import type { Task } from "@/api";

import { probeTaskFeedback } from "../probe-task-feedback";

function task(overrides: Partial<Task>): Task {
  return {
    id: "probe-1",
    skill: "sub2api-connectivity-test",
    operation: "active-probe",
    status: "succeeded",
    progress: 100,
    message: "官方探测完成",
    result: {},
    created_at: "2026-09-09T00:00:00Z",
    updated_at: "2026-09-09T00:00:01Z",
    ...overrides,
  };
}

describe("探活任务结果提示", () => {
  it("单账号收到模型响应时显示探活通过和请求耗时", () => {
    expect(
      probeTaskFeedback(
        task({
          result: { results: [{ result: "通过", duration_ms: 842 }] },
        }),
        "AI Relay-0.6",
      ),
    ).toEqual({
      tone: "success",
      title: "AI Relay-0.6：探活通过",
      description: "耗时 842 毫秒",
    });
  });

  it("单账号请求失败时显示探活失败、请求耗时和失败原因", () => {
    expect(
      probeTaskFeedback(
        task({
          status: "failed",
          result: {
            results: [
              {
                result: "失败",
                duration_ms: 2180,
                failure_reason: "Upstream authentication failed",
              },
            ],
          },
        }),
        "AI Relay-0.6",
      ),
    ).toEqual({
      tone: "error",
      title: "AI Relay-0.6：探活失败",
      description: "耗时 2.18 秒 · Upstream authentication failed",
    });
  });

  it("批量探活有失败项时显示部分通过及汇总耗时", () => {
    expect(
      probeTaskFeedback(
        task({
          status: "partial",
          result: { passed: 1, failed: 1, skipped: 0, duration_ms: 3210, results: [{}, {}] },
        }),
      ),
    ).toEqual({
      tone: "warning",
      title: "探活部分通过",
      description: "通过 1，失败 1，跳过 0 · 耗时 3.21 秒",
    });
  });

  it("旧探活任务没有请求耗时时使用任务起止时间", () => {
    expect(
      probeTaskFeedback(
        task({
          status: "succeeded",
          result: {
            results: [{ result: "失败", failure_reason: "Upstream authentication failed" }],
          },
          updated_at: "2026-09-09T00:00:02.180Z",
        }),
        "AI Relay-0.6",
      ),
    ).toMatchObject({
      tone: "error",
      title: "AI Relay-0.6：探活失败",
      description: "耗时 2.18 秒 · Upstream authentication failed",
    });
  });
});
