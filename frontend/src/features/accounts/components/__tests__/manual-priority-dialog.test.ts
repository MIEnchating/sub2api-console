import { describe, expect, it } from "vitest";

import type { AccountStatus } from "@/api";
import { manualPriorityInitialValues, manualPrioritySlots } from "../manual-priority-dialog";

describe("手动控制选择", () => {
  it("展示共同分组账号使用的位置并保留当前账号自己的选择", () => {
    const accounts = [
      {
        id: "41",
        name: "当前账号",
        groups: ["codex", "shared"],
        manual_priority: 2,
      },
      { id: "42", name: "同组账号", groups: ["codex"], manual_priority: 3 },
      {
        id: "43",
        name: "其他分组账号",
        groups: ["claude"],
        manual_priority: 1,
      },
    ] as AccountStatus[];

    const slots = manualPrioritySlots(accounts, "41", 4);

    expect(slots.map((slot) => [slot.priority, slot.owner?.id ?? null, slot.count])).toEqual([
      [1, null, 0],
      [2, null, 0],
      [3, "42", 1],
      [4, null, 0],
    ]);
  });

  it("多分组账号按最拥挤的共同分组计算位置容量", () => {
    const accounts = [
      { id: "41", groups: ["codex", "shared"], manual_priority: null },
      { id: "42", groups: ["codex", "shared"], manual_priority: 3 },
      { id: "43", groups: ["shared"], manual_priority: 3 },
      { id: "44", groups: ["shared"], manual_priority: 3 },
      { id: "45", groups: ["other"], manual_priority: 3 },
    ] as AccountStatus[];

    expect(manualPrioritySlots(accounts, "41", 10)[2]?.count).toBe(3);
  });

  it("首次设置默认负载因子和并发上限都是 100，调整时回显现值", () => {
    expect(
      manualPriorityInitialValues({
        id: "41",
        manual_priority: null,
        load_factor: "2",
        concurrency: 3,
      } as AccountStatus),
    ).toEqual({
      loadFactor: "100",
      concurrency: 100,
      schedulable: true,
      syncBalanceMultiplier: true,
    });
    expect(
      manualPriorityInitialValues({
        id: "42",
        manual_priority: 3,
        load_factor: "80",
        concurrency: 120,
        schedulable: false,
        manual_sync_balance_multiplier: true,
      } as AccountStatus),
    ).toEqual({
      loadFactor: "80",
      concurrency: 120,
      schedulable: false,
      syncBalanceMultiplier: true,
    });
  });
});
