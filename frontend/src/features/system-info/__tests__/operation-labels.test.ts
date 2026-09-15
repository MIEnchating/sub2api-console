import { expect, it } from "vitest";
import { taskOperationLabel } from "../constants";

it.each(["constructor", "__proto__", "toString"])("未识别任务操作%s按原协议值展示", (operation) => {
  expect(taskOperationLabel(operation)).toBe(operation);
});
