import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { SetupPage } from "../App";

it("初始化表单为每个必填凭据及管理地址提供可访问名称", () => {
  render(
    <SetupPage
      status={{ initialized: false, target_configured: false, setup_token_required: true }}
      onComplete={vi.fn()}
    />,
  );
  for (const label of [
    "控制台账号",
    "控制台密码",
    "确认控制台密码",
    "初始化令牌",
    "Admin Base URL",
    "Admin Key",
  ]) {
    expect(screen.getByLabelText(label, { exact: true })).toBeVisible();
  }
});
