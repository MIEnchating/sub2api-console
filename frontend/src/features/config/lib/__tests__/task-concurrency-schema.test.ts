import { expect, it } from "vitest";
import { taskConcurrencySchema } from "../task-concurrency-schema";

it.each(["", "0", "-1", "1.5", "10001", "abc"])("并发或队列填入 %s 时拒绝提交", (value) => {
  expect(
    taskConcurrencySchema.safeParse({
      limits: { account: value },
      queueCapacity: "100",
      version: "v1",
    }).success,
  ).toBe(false);
  expect(
    taskConcurrencySchema.safeParse({
      limits: { account: "8" },
      queueCapacity: value,
      version: "v1",
    }).success,
  ).toBe(false);
});
it("允许边界整数并保留版本及每个模块", () => {
  const input = { limits: { account: "1", probe: "10000" }, queueCapacity: "100", version: "v1" };
  expect(taskConcurrencySchema.parse(input)).toEqual(input);
});
