import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { Task } from "@/api";
import { ModelCheckResult } from "../model-check-result";

afterEach(cleanup);

it.each([
  ["subscription", "订阅特征", 2, 4],
  ["official_key", "官 Key 特征", 4, 10],
  ["inconclusive", "来源无法判定", 2, 10],
])("Astra 返回 %s 时在桌面和移动端展示来源及两档数值", (source, label, low, mid) => {
  const task: Task = {
    id: "astra-task",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status: "succeeded",
    progress: 100,
    message: "检测完成",
    created_at: "",
    updated_at: "",
    result: {
      tests: [
        {
          account_id: "41",
          account_name: "测试账号",
          claimed_model: "gpt-6-astra",
          checker: "astra",
          verdict: "MATCH",
          access_source: source,
          identity_passed: 2,
          identity_total: 2,
          requests: { successful: 4, total: 4 },
          checks: [
            { round: 1, id: "juice-low", number: low },
            { round: 1, id: "juice-mid", number: mid },
          ],
        },
      ],
    },
  };
  render(<ModelCheckResult task={task} />);
  for (const id of ["model-check-result-desktop-table", "model-check-result-mobile-list"]) {
    const result = screen.getByTestId(id);
    expect(result).toHaveTextContent(String(label));
    expect(result).toHaveTextContent(`low：${low}`);
    expect(result).toHaveTextContent(`mid：${mid}`);
    expect(result).toHaveTextContent("题目通过 2/2");
    expect(within(result).queryByRole("progressbar")).not.toBeInTheDocument();
  }
});
