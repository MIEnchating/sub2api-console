import { expect, it } from "vitest";
import { taskOperationLabel } from "../constants";

it.each(["constructor", "__proto__", "toString"])("未识别任务操作%s按原协议值展示", (operation) => {
  expect(taskOperationLabel(operation)).toBe(operation);
});

it("渠道批量任务显示中文操作名称", () => {
  expect(taskOperationLabel("newapi-channel-models")).toBe("渠道模型批量维护");
});
