import { describe, expect, it } from "vitest";

import { terminalRefreshKeys } from "../task-refresh";

describe("terminal task refresh contract", () => {
  it("refreshes affected data after a failed task as well as a successful task", () => {
    const failed = terminalRefreshKeys("management-sync", { status: "failed" } as never);
    const succeeded = terminalRefreshKeys("management-sync", { status: "succeeded" } as never);

    expect(failed).toEqual([
      ["accounts"],
      ["groups"],
      ["upstreams"],
      ["logs"],
      ["overview"],
      ["overview-events"],
    ]);
    expect(succeeded).toEqual(failed);
  });

  it("does not refresh while the task is queued or running", () => {
    expect(terminalRefreshKeys("alerts", { status: "queued" } as never)).toEqual([]);
    expect(terminalRefreshKeys("alerts", { status: "running" } as never)).toEqual([]);
  });

  it("refreshes auth configuration when an upstream task waits for operator input", () => {
    expect(
      terminalRefreshKeys("upstream-action", { status: "waiting_input" } as never),
    ).toContainEqual(["auth-recovery-config"]);
  });

  it("refreshes live catalogs and logs after upstream and onboarding tasks", () => {
    expect(terminalRefreshKeys("upstream-action", { status: "succeeded" } as never)).toContainEqual(
      ["upstream-groups"],
    );
    expect(terminalRefreshKeys("onboarding", { status: "succeeded" } as never)).toContainEqual([
      "accounts",
    ]);
    expect(terminalRefreshKeys("alerts", { status: "failed" } as never)).toContainEqual(["logs"]);
  });

  it("refreshes scheduler and account projections after an inspection", () => {
    const keys = terminalRefreshKeys("inspection", { status: "succeeded" } as never);

    expect(keys).toContainEqual(["accounts"]);
    expect(keys).toContainEqual(["auto-inspection"]);
    expect(keys).toContainEqual(["logs"]);
  });

  it("updates account evidence after a standalone probe", () => {
    expect(terminalRefreshKeys("active-probe", { status: "failed" } as never)).toEqual([
      ["accounts"],
      ["logs"],
      ["overview-events"],
    ]);
  });

  it("refreshes local policy after a dedicated account control task", () => {
    const keys = terminalRefreshKeys("account-scheduling", { status: "succeeded" } as never);

    expect(keys).toContainEqual(["accounts"]);
    expect(keys).toContainEqual(["policy"]);
    expect(keys).toContainEqual(["logs"]);
  });
});
