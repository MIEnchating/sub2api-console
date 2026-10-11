import { createRoot } from "react-dom/client";

import type { Task } from "../../../src/api";
import { TooltipProvider } from "../../../src/components/ui/tooltip";
import { ModelCheckResult } from "../../../src/features/model-check/components/model-check-result";

const status =
  new URLSearchParams(window.location.search).get("status") === "failed" ? "failed" : "cancelled";
const task: Task = {
  id: "isolated-interrupted-check",
  skill: "sub2api-model-check",
  operation: "account-model-behavior-check",
  status,
  progress: 100,
  message: "检测连接中断，请重新提交",
  created_at: "2026-10-10T00:00:00Z",
  updated_at: "2026-10-10T00:00:01Z",
  result: {
    completed: 21,
    total: 23,
    profile_version: "rules-isolated",
    tests: Array.from({ length: 21 }, (_, index) => ({
      account_id: String(index + 1),
      account_name: `已完成账号 ${index + 1}`,
      claimed_model: "gpt-6-astra",
      checker: "astra",
      verdict: "MATCH",
      requests: { successful: 3, total: 3 },
      identity_passed: 3,
      identity_total: 3,
      response_models: ["gpt-6-astra"],
    })),
  },
};

createRoot(document.getElementById("result-root")!).render(
  <TooltipProvider>
    <ModelCheckResult task={task} />
  </TooltipProvider>,
);
