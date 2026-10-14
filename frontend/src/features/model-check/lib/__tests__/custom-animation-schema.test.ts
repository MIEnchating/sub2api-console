import { expect, it } from "vitest";
import { customAnimationSchema } from "../animation-schema";

it("Gemini 自定义接口保留多模型与原生地址并通过表单校验", () => {
  const input = {
    platform: "gemini",
    base_url: "https://example.test/v1beta",
    api_key: "test-key",
    model: "gemini-test",
    additional_models: ["gemini-other"],
    timeout_seconds: 120,
  };
  expect(customAnimationSchema.parse(input)).toMatchObject(input);
});
