import { expect, it } from "vitest";
import { createSetupSchema } from "../setup-schema";

it.each([true, false])("已有目标=%s时本地模式不会校验未提交的隐藏目标", (targetConfigured) => {
  const value = createSetupSchema(false, targetConfigured).safeParse({
    username: "operator",
    password: "private-password",
    confirm_password: "private-password",
    local_export_only: !targetConfigured,
    admin_base_url: "unfinished",
    admin_key: "",
    setup_token: "",
  });
  expect(value.success).toBe(true);
});

it.each([
  "http:isolated.example",
  "https://isolated.example?",
  "https://isolated.example#",
  "https://user:pass@isolated.example",
])("初始化管理地址 %s 与后端限制一致", (url) => {
  expect(
    createSetupSchema(false, false).safeParse({
      username: "operator",
      password: "private-password",
      confirm_password: "private-password",
      local_export_only: false,
      admin_base_url: url,
      admin_key: "test-key",
      setup_token: "",
    }).success,
  ).toBe(false);
});
