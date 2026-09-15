import { describe, expect, it } from "vitest";
import { maxInputBytes } from "../constants";
import { oauthBatchDefaults, oauthBatchSchema } from "../lib/oauth-batch-schema";

describe("批量授权输入", () => {
  it("空账号内容不能提交", () => {
    const result = oauthBatchSchema.safeParse(oauthBatchDefaults);
    expect(result.success).toBe(false);
    if (!result.success) expect(result.error.issues[0].path).toEqual(["content"]);
  });
  it("超过2MB的多字节内容按字节数拒绝", () => {
    expect(
      oauthBatchSchema.safeParse({ ...oauthBatchDefaults, content: "账".repeat(maxInputBytes / 2) })
        .success,
    ).toBe(false);
  });
  it("共享接码未确认费用时不能解析授权", () => {
    const result = oauthBatchSchema.safeParse({
      ...oauthBatchDefaults,
      content: "user@example.test",
      sms_provider: "custom",
      sms_custom_entries: "+12025550123----https://sms.example.test/code",
    });
    expect(result.success).toBe(false);
    if (!result.success) expect(result.error.issues[0].path).toEqual(["sms_confirmed"]);
  });
  it("有效批量文本与已确认共享接码可以提交", () => {
    expect(
      oauthBatchSchema.safeParse({
        ...oauthBatchDefaults,
        content: "user@example.test----password",
        sms_provider: "custom",
        sms_custom_entries: "+12025550123----https://sms.example.test/code",
        sms_confirmed: true,
      }).success,
    ).toBe(true);
  });
});
