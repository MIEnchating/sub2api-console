import type { QueryClient } from "@tanstack/react-query";
import { fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { boundCandidate, renderOnboarding } from "./onboarding-fixture";

let client: QueryClient | undefined;
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("添加新上游时地址、凭据和类型均可通过可见标签访问", async () => {
  client = renderOnboarding();
  await screen.findByRole("button", { name: "添加并验证上游" });
  for (const label of [
    "上游地址",
    "账号 Base URL",
    "名称",
    "上游类型",
    "鉴权方式",
    "充值比例",
    "密码箱密码项",
  ]) {
    expect(screen.getByLabelText(label, { exact: true })).toBeVisible();
  }
  expect(screen.getByRole("combobox", { name: "上游地址协议" })).toBeVisible();
});

it("已有绑定分组的成本与创建参数提供可访问名称", async () => {
  client = renderOnboarding(boundCandidate("active"));
  await screen.findByRole("button", { name: "预览更新绑定" });
  for (const label of ["账号成本（已换算）", "并发", "优先级", "备注"]) {
    expect(screen.getByLabelText(label, { exact: true })).toBeVisible();
  }
});

it("批量开户的参数提供可访问名称", async () => {
  client = renderOnboarding(boundCandidate("active"), false);
  await screen.findByRole("button", { name: "预览 0 项变更" });
  for (const label of ["批量备注（可选）", "并发", "优先级"]) {
    expect(screen.getByLabelText(label, { exact: true })).toBeVisible();
  }
});

it("启用自定义请求头后编辑框提供明确名称", async () => {
  client = renderOnboarding();
  fireEvent.click(await screen.findByRole("switch", { name: /自定义请求头/ }));
  expect(screen.getByRole("textbox", { name: "自定义请求头 JSON" })).toBeVisible();
});

it.each([
  ["Token + 刷新 Token", ["Token", "刷新 Token"]],
  ["自定义账号密码", ["用户名", "密码"]],
])("切换为 %s 鉴权后凭据输入框关联标签", async (mode, labels) => {
  const user = userEvent.setup();
  client = renderOnboarding();
  await user.click(await screen.findByRole("combobox", { name: "鉴权方式" }));
  await user.click(await screen.findByRole("option", { name: mode }));
  for (const label of labels) expect(screen.getByLabelText(label, { exact: true })).toBeVisible();
});
