import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { Task } from "@/api";

import { ModelCheckResult } from "../model-check-result";

function task(tests: Record<string, unknown>[]): Task {
  return {
    id: "model-check-1",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status: "succeeded",
    progress: 100,
    message: "账号模型检测完成",
    result: { tests, summary: { SOL_CONSISTENT: 1, MISMATCH: 1 } },
    created_at: "2026-08-28T00:00:00Z",
    updated_at: "2026-08-28T00:00:01Z",
  };
}

describe("模型检测结果", () => {
  it("任务运行中展示实时步骤和已完成组合而不是百分比", () => {
    const runningTask = {
      ...task([]),
      status: "running",
      progress: 14,
      message: "已完成 2/20 个账号模型组合",
      result: {
        phase: "testing",
        completed: 2,
        total: 20,
        tests: [
          {
            account_id: "41",
            account_name: "实时账号",
            claimed_model: "gpt-5.6-sol",
            verdict: "SOL_CONSISTENT",
            requests: { successful: 2, total: 2 },
          },
        ],
      },
    } as Task;
    const markup = renderToStaticMarkup(<ModelCheckResult task={runningTask} />);

    expect(markup).toContain('role="status"');
    expect(markup).toContain('data-testid="model-check-live-progress"');
    expect(markup).toContain("已完成 2/20 个账号模型组合");
    expect(markup).toContain("2/20");
    expect(markup).toContain("准备账号凭据");
    expect(markup).toContain("并行执行检测");
    expect(markup).toContain("实时账号");
    expect(markup).not.toContain("14%");
    expect(markup).not.toContain('data-testid="model-check-result"');
    expect(markup).toContain('data-table-panel=""');
  });

  it("按账号和模型组合展示 Sol 与 Claude 结果", () => {
    const markup = renderToStaticMarkup(
      <ModelCheckResult
        task={task([
          {
            account_id: "41",
            account_name: "Sol 账号",
            checker: "sol",
            verdict: "SOL_CONSISTENT",
            claimed_model: "gpt-5.6-sol",
            similarity_percent: { sol: 81.2, luna: 10.3, terra: 8.5 },
            coverage: { percent: 95.8 },
            requests: { successful: 2, total: 2 },
            response_models: ["gpt-5.6-sol"],
          },
          {
            account_id: "42",
            account_name: "Claude 账号",
            checker: "claude",
            verdict: "MISMATCH",
            claimed_model: "claude-opus-5",
            identity_match_percent: 12.4,
            coverage_percent: 100,
            requests: { successful: 6, total: 6 },
            response_models: ["rewritten-model"],
          },
        ])}
      />,
    );

    expect(markup).toContain("2 个账号模型组合");
    expect(markup).toContain("Sol 账号");
    expect(markup).toContain("Claude 账号");
    expect(markup).toContain("符合 Sol 行为");
    expect(markup).toContain("不匹配");
    expect(markup).toContain("81.2%");
    expect(markup).toContain("12.4%");
    expect(markup).toContain("2/2");
    expect(markup).toContain("rewritten-model");
    expect(markup).toContain('data-table-panel=""');
  });

  it("零成功请求显示请求失败并隐藏无意义的相似度", () => {
    const markup = renderToStaticMarkup(
      <ModelCheckResult
        task={task([
          {
            account_id: "14",
            account_name: "失败账号",
            checker: "sol",
            verdict: "INCONCLUSIVE",
            claimed_model: "gpt-5.6-sol",
            similarity_percent: { sol: 33.3, luna: 33.3, terra: 33.3 },
            coverage: { percent: 0 },
            requests: { successful: 0, total: 4 },
            response_models: ["gpt-5.6-sol"],
          },
        ])}
      />,
    );

    expect(markup).toContain("请求失败");
    expect(markup).toContain("0/4");
    expect(markup).toContain("所有检测请求均失败");
    expect(markup).not.toContain("33.3%");
    expect(markup).not.toContain("无法判定");
  });

  it("默认每页展示十条并显示完整结果总数", () => {
    const tests = Array.from({ length: 12 }, (_, index) => ({
      account_id: `${index + 1}`,
      account_name: `分页账号 ${index + 1}`,
      checker: "sol",
      verdict: "SOL_CONSISTENT",
      claimed_model: `gpt-page-${index + 1}`,
      similarity_percent: { sol: 90 },
      coverage: { percent: 100 },
      requests: { successful: 2, total: 2 },
    }));
    const markup = renderToStaticMarkup(<ModelCheckResult task={task(tests)} />);

    expect(markup).toContain("12 个账号模型组合");
    expect(markup).toContain("分页账号 10");
    expect(markup).not.toContain("分页账号 11");
    expect(markup).toContain(">12</span>");
    expect(markup).toContain('aria-label="转到第 2 页"');
  });
});
