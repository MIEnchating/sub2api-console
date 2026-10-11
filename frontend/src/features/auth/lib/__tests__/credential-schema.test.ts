import { expect, it } from "vitest";
import { consolePasswordSchema, consoleUsernameSchema } from "../credential-schema";

it("账号按Unicode字符限制长度，去除首尾空白", () => {
  expect(consoleUsernameSchema.parse(`  ${"😀".repeat(80)}  `)).toBe("😀".repeat(80));
  expect(consoleUsernameSchema.safeParse("😀").success).toBe(false);
  expect(consoleUsernameSchema.safeParse("😀".repeat(81)).success).toBe(false);
});

it("密码按Unicode字符校验并完整保留空白", () => {
  const schema = consolePasswordSchema();
  expect(schema.parse("😀".repeat(256))).toBe("😀".repeat(256));
  expect(schema.parse("  password  ")).toBe("  password  ");
  expect(schema.safeParse("😀".repeat(9)).success).toBe(false);
  expect(schema.safeParse("😀".repeat(257)).success).toBe(false);
});
