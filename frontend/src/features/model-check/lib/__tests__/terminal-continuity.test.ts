import { expect, it } from "vitest";
import type { Task } from "@/api";
import { terminalContinuityBusyIDs, terminalContinuityResults } from "../terminal-continuity";

function task(id: string, accountID: string, status: Task["status"], createdAt: string): Task {
  return {
    id,
    skill: "sub2api-terminal-continuity",
    operation: "account-terminal-continuity",
    status,
    progress: status === "succeeded" ? 100 : 0,
    message: "",
    created_at: createdAt,
    updated_at: createdAt,
    result: {
      account_ids: [accountID],
      checks:
        status === "succeeded"
          ? [
              {
                account_id: accountID,
                account_name: accountID,
                model: "model",
                request_id: id,
                verdict: id === "new" ? "suspected" : "normal",
                response: "response",
                duration_ms: 10,
                completed_at: createdAt,
              },
            ]
          : [],
    },
  };
}

it("同一账号保留最新独立检测结论且不混入旧结果", () => {
  const results = terminalContinuityResults([
    task("old", "41", "succeeded", "2026-09-21T00:00:00Z"),
    task("new", "41", "succeeded", "2026-09-22T00:00:00Z"),
  ]);
  expect(results.get("41")?.verdict).toBe("suspected");
});

it("只把独立检测中任务的稳定账号 ID 标记为忙碌", () => {
  expect(
    terminalContinuityBusyIDs([
      task("running", "41", "running", "2026-09-22T00:00:00Z"),
      task("done", "42", "succeeded", "2026-09-22T00:00:00Z"),
    ]),
  ).toEqual(new Set(["41"]));
});
