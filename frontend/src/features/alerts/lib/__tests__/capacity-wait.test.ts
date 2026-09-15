import { expect, it } from "vitest";

import { alertDeliveryLabel, alertStatusLabel } from "../alert-display";

it("旧容量等待告警关闭后显示等待原因而非未知通知状态", () => {
  expect(alertStatusLabel("closed")).toBe("已关闭");
  expect(alertDeliveryLabel("等待并发额度，已转为调度等待记录")).toBe("等待并发额度，不发送通知");
});
