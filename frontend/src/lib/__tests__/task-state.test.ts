import { describe, expect, it } from "vitest";
import { taskIsPending, taskIsTerminal, taskPollInterval } from "../task-state";

describe("task state helpers", () => {
  it("keeps a queued or running task pending", () => {
    expect(taskIsPending("task-1", { isError: false, data: { status: "queued" } as never })).toBe(
      true,
    );
    expect(taskIsPending("task-1", { isError: false, data: { status: "running" } as never })).toBe(
      true,
    );
  });

  it("releases controls after a terminal task or a task query error", () => {
    expect(
      taskIsPending("task-1", { isError: false, data: { status: "succeeded" } as never }),
    ).toBe(false);
    expect(taskIsPending("task-1", { isError: false, data: { status: "failed" } as never })).toBe(
      false,
    );
    expect(
      taskIsPending("task-1", { isError: false, data: { status: "cancelled" } as never }),
    ).toBe(false);
    expect(
      taskIsPending("task-1", { isError: false, data: { status: "waiting_input" } as never }),
    ).toBe(false);
    expect(taskIsPending("task-1", { isError: true })).toBe(true);
  });

  it("recognizes completed, failed and cancelled tasks as terminal states", () => {
    expect(taskIsTerminal({ status: "succeeded" } as never)).toBe(true);
    expect(taskIsTerminal({ status: "failed" } as never)).toBe(true);
    expect(taskIsTerminal({ status: "cancelled" } as never)).toBe(true);
    expect(taskIsTerminal({ status: "waiting_input" } as never)).toBe(false);
    expect(taskIsTerminal({ status: "running" } as never)).toBe(false);
    expect(taskIsTerminal()).toBe(false);
  });

  it("stops polling after a terminal result or a task status request error", () => {
    expect(taskPollInterval({ state: { status: "error" } })).toBe(2_000);
    expect(
      taskPollInterval({ state: { status: "success", data: { status: "failed" } as never } }),
    ).toBe(false);
    expect(
      taskPollInterval({
        state: { status: "success", data: { status: "waiting_input" } as never },
      }),
    ).toBe(false);
    expect(
      taskPollInterval({ state: { status: "success", data: { status: "running" } as never } }, 300),
    ).toBe(300);
  });
});
