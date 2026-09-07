import { describe, expect, it } from "vitest";
import { taskIsPending, taskIsTerminal, taskPollInterval, taskStopsPolling } from "../task-state";

describe("task state helpers", () => {
  it("keeps a queued or running task pending", () => {
    expect(taskIsPending("task-1", { isError: false, data: { status: "queued" } as never })).toBe(
      true,
    );
    expect(taskIsPending("task-1", { isError: false, data: { status: "running" } as never })).toBe(
      true,
    );
  });

  it("releases controls only after a terminal task", () => {
    expect(
      taskIsPending("task-1", { isError: false, data: { status: "succeeded" } as never }),
    ).toBe(false);
    expect(taskIsPending("task-1", { isError: false, data: { status: "failed" } as never })).toBe(
      false,
    );
    expect(taskIsPending("task-1", { isError: false, data: { status: "partial" } as never })).toBe(
      false,
    );
    expect(
      taskIsPending("task-1", { isError: false, data: { status: "cancelled" } as never }),
    ).toBe(false);
    expect(
      taskIsPending("task-1", { isError: false, data: { status: "waiting_input" } as never }),
    ).toBe(true);
    expect(taskIsPending("task-1", { isError: true })).toBe(true);
  });

  it("recognizes completed, partially failed, failed and cancelled tasks as terminal states", () => {
    expect(taskIsTerminal({ status: "succeeded" } as never)).toBe(true);
    expect(taskIsTerminal({ status: "partial" } as never)).toBe(true);
    expect(taskIsTerminal({ status: "failed" } as never)).toBe(true);
    expect(taskIsTerminal({ status: "cancelled" } as never)).toBe(true);
    expect(taskIsTerminal({ status: "waiting_input" } as never)).toBe(false);
    expect(taskIsTerminal({ status: "running" } as never)).toBe(false);
    expect(taskIsTerminal()).toBe(false);
  });

  it("keeps waiting-input tasks active for controls and status refresh", () => {
    expect(taskStopsPolling({ status: "waiting_input" } as never)).toBe(false);
    expect(taskStopsPolling({ status: "cancelled" } as never)).toBe(true);
  });

  it("keeps polling errors and waiting-input tasks at a reduced rate", () => {
    expect(taskPollInterval({ state: { status: "error" } })).toBe(2_000);
    expect(
      taskPollInterval({ state: { status: "success", data: { status: "failed" } as never } }),
    ).toBe(false);
    expect(
      taskPollInterval({ state: { status: "success", data: { status: "partial" } as never } }),
    ).toBe(false);
    expect(
      taskPollInterval({
        state: { status: "success", data: { status: "waiting_input" } as never },
      }),
    ).toBe(2_000);
    expect(
      taskPollInterval({ state: { status: "success", data: { status: "running" } as never } }, 300),
    ).toBe(300);
  });
});
