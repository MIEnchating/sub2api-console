import { expect, it } from "vitest";
import { templateDefaults } from "../template-schema";
import type { KumaTemplate } from "@/api";

it("编辑接口返回请求内容时回显原值，鉴权密码保持空白", () => {
  const item = {
    id: "a".repeat(48),
    name: "API",
    revision: 3,
    headers: '{"X-Key":"saved-key"}',
    body: '{"input":"ping"}',
    auth_password: "must-not-use",
  } as KumaTemplate & { headers: string; body: string; auth_password: string };
  const value = templateDefaults(item);
  expect(value.headers).toBe(item.headers);
  expect(value.body).toBe(item.body);
  expect(value.auth_password).toBe("");
});
