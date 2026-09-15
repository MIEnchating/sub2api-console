import { expect, it } from "vitest";
import { createMonitorSchema, defaultMonitorOptions } from "../schemas";

it.each(["basic", "bearer"])("新增 %s 鉴权但未填写凭据时在对应字段拒绝提交", (method) => {
  const result = createMonitorSchema().safeParse({
    name: "接口",
    type: "http",
    url: "https://monitor.example",
    interval: 60,
    parent: null,
    options: { ...defaultMonitorOptions, auth_method: method },
  });
  expect(result.success).toBe(false);
  if (!result.success) {
    const paths = result.error.issues.map((issue) => issue.path.join("."));
    expect(paths).toContain("options.auth_password");
    if (method === "basic") expect(paths).toContain("options.auth_username");
  }
});

it("新增 Basic 鉴权并填写完整凭据时允许提交", () => {
  expect(
    createMonitorSchema().safeParse({
      name: "接口",
      type: "http",
      url: "https://monitor.example",
      interval: 60,
      parent: null,
      options: {
        ...defaultMonitorOptions,
        auth_method: "basic",
        auth_username: "monitor",
        auth_password: "secret",
      },
    }).success,
  ).toBe(true);
});

it.each(["basic", "bearer", "ntlm"])("编辑已有 %s 鉴权时切换模板允许凭据留空", (method) => {
  expect(
    createMonitorSchema(method).safeParse({
      name: "接口",
      type: "http",
      url: "",
      interval: 60,
      parent: null,
      template_id: "a".repeat(48),
      template_auth_override: true,
      options: { ...defaultMonitorOptions, auth_method: method },
    }).success,
  ).toBe(true);
});

it("编辑切换鉴权方式必须填写新凭据，不借用原鉴权凭据", () => {
  const result = createMonitorSchema("bearer").safeParse({
    name: "接口",
    type: "http",
    url: "",
    interval: 60,
    parent: null,
    options: { ...defaultMonitorOptions, auth_method: "basic" },
  });
  expect(result.success).toBe(false);
  if (!result.success)
    expect(result.error.issues.map((issue) => issue.path.join("."))).toEqual([
      "options.auth_username",
      "options.auth_password",
    ]);
});

it("编辑 Basic 仅更新密码时允许用户名留空保留", () => {
  expect(
    createMonitorSchema("basic").safeParse({
      name: "接口",
      type: "http",
      url: "",
      interval: 60,
      parent: null,
      template_id: "a".repeat(48),
      template_auth_override: true,
      options: { ...defaultMonitorOptions, auth_method: "basic", auth_password: "replacement" },
    }).success,
  ).toBe(true);
});
