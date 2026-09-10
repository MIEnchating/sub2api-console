import { describe, expect, it } from "vitest";
import { templateDefaults, templateSchema } from "../template-schema";
import { parseRequestProfile } from "../request-profiles";
import type { KumaTemplate } from "@/api";

describe("模板请求模式", () => {
  it.each(["claude-messages", "openai-chat", "openai-responses", "claude-cli"])(
    "编辑 %s 模板时保留原有模式和模型",
    (profile) => {
      const values = templateDefaults({
        request_profile: profile,
        model: "selected-model",
        name: "API",
      } as KumaTemplate);
      expect(values.request_profile).toBe(profile);
      expect(values.model).toBe("selected-model");
      expect(templateSchema.safeParse(values).success).toBe(true);
    },
  );
  it("非法请求模式无法提交", () => {
    expect(
      templateSchema.safeParse({
        ...templateDefaults(),
        name: "API",
        request_profile: "openai-invalid",
      }).success,
    ).toBe(false);
    expect(parseRequestProfile("openai-invalid")).toBe("");
  });
});
