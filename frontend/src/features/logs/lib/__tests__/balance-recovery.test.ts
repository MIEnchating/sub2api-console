import { expect, it } from "vitest";
import { logTitleLabel } from "../log-display";

it("余额恢复审计和失败事件使用中文名称", () => {
  expect(logTitleLabel("account.balance_recovery")).toBe("余额恢复后恢复账号");
  expect(logTitleLabel("balance_recovery_failed")).toBe("余额恢复后清理账号状态失败");
});
