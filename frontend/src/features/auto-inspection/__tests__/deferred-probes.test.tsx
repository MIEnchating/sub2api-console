import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { AutoInspectionHeartbeatDetails } from "../../../App";
import type { AutoInspectionStatus, Task } from "../../../api";

const heartbeat: AutoInspectionStatus["heartbeat_history"][number] = {
  checked_at: "2026-09-14T08:00:00Z",
  completed_at: "2026-09-14T08:00:30Z",
  status: "succeeded",
  operations: ["active_probe"],
  operation_timings: [{ operation: "active_probe", duration_seconds: 30 }],
  task_id: "inspection-batch",
  error: null,
  skipped: false,
};

const task: Task = {
  id: "inspection-batch",
  skill: "sub2api-auto-inspection",
  operation: "automatic-inspection",
  status: "succeeded",
  progress: 100,
  message: "巡检完成",
  result: {},
  created_at: "2026-09-14T08:00:00Z",
  updated_at: "2026-09-14T08:00:30Z",
};

describe("主动探测分批摘要", () => {
  it("到期账号因批量上限留待后续巡检时将账号数放在摘要开头", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={heartbeat}
        task={{
          ...task,
          result: {
            evidence: {
              monitored_accounts: 30,
              traffic_persisted: 12,
              probes_persisted: 8,
              probes_deferred: 22,
            },
          },
        }}
      />,
    );

    expect(screen.getByText(/^待后续巡检 22 个账号；监控 /)).toHaveTextContent(
      "监控 30 个账号，新增 12 条流量样本、8 条探测样本",
    );
  });

  it.each([undefined, 0])("待后续巡检数为 %s 时保留原有完成摘要", (deferred) => {
    render(
      <AutoInspectionHeartbeatDetails
        record={heartbeat}
        task={{
          ...task,
          result: {
            evidence: {
              monitored_accounts: 8,
              traffic_persisted: 12,
              probes_persisted: 8,
              probes_deferred: deferred,
            },
          },
        }}
      />,
    );

    expect(screen.getByText("监控 8 个账号，新增 12 条流量样本、8 条探测样本")).toBeInTheDocument();
    expect(screen.queryByText(/待后续巡检 \d+ 个账号/)).not.toBeInTheDocument();
  });
});
