import { expect, it } from "vitest";
import { customAnimationSchema } from "../animation-schema";

const valid = {
  base_url: "https://example.invalid/proxy/v1",
  api_key: "isolated-secret",
  model: "test-model",
  platform: "openai",
  timeout_seconds: 120,
};

it("自定义接口包含路径和有效 Key 时保留路径并清理输入首尾空白", () => {
  expect(
    customAnimationSchema.parse({ ...valid, api_key: " isolated-secret ", model: " test-model " }),
  ).toEqual(valid);
});

it.each([
  "",
  "file:///tmp/test",
  "https://user:pass@example.invalid",
  "https://example.invalid?key=secret",
  "https://example.invalid#secret",
  "https://example.invalid?",
])("Base URL 为 %s 时拒绝提交", (base_url) => {
  expect(customAnimationSchema.safeParse({ ...valid, base_url }).success).toBe(false);
});

it.each([
  { name: "空值", api_key: "" },
  { name: "换行注入", api_key: "key\r\nheader" },
  { name: "空白", api_key: "key with spaces" },
  { name: "超长", api_key: "k".repeat(4097) },
])("Key 包含 $name 时拒绝提交", ({ api_key }) => {
  expect(customAnimationSchema.safeParse({ ...valid, api_key }).success).toBe(false);
});

it("模型字段误填 Key 时拒绝提交，防止凭据进入检测记录", () => {
  expect(customAnimationSchema.safeParse({ ...valid, model: valid.api_key }).success).toBe(false);
});
