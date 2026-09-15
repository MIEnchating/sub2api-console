import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SetupPage } from "../App";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("管理地址尚未填写完成时切换本地初始化可提交且不发送隐藏的管理凭据", async () => {
  const fetcher = vi.fn<typeof fetch>(async () =>
    Response.json({ initialized: true, target_configured: false }),
  );
  vi.stubGlobal("fetch", fetcher);
  const complete = vi.fn();
  render(
    <SetupPage
      status={{ initialized: false, target_configured: false, setup_token_required: false }}
      onComplete={complete}
    />,
  );
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("控制台账号", { exact: true }), "local-operator");
  await user.type(screen.getByLabelText("控制台密码", { exact: true }), "private-local-password");
  await user.type(
    screen.getByLabelText("确认控制台密码", { exact: true }),
    "private-local-password",
  );
  await user.type(screen.getByLabelText("Admin Base URL", { exact: true }), "unfinished-target");
  await user.type(screen.getByLabelText("Admin Key", { exact: true }), "private-managed-key");
  await user.click(
    screen.getByRole("checkbox", { name: "仅使用本地账号工作台（不配置线上管理目标）" }),
  );
  expect(screen.queryByLabelText("Admin Base URL", { exact: true })).not.toBeInTheDocument();
  expect(screen.queryByLabelText("Admin Key", { exact: true })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "完成初始化" }));
  await waitFor(() => expect(complete).toHaveBeenCalledOnce());
  const request = fetcher.mock.calls[0];
  expect(request?.[0]).toBe("/api/setup/initialize");
  expect(JSON.parse(String(request?.[1]?.body))).toEqual({
    username: "local-operator",
    password: "private-local-password",
    local_export_only: true,
    admin_base_url: "",
    admin_key: "",
  });
  expect(request?.[1]?.credentials).toBe("include");
});

it("本地初始化仍校验确认密码并可用键盘切回管理目标配置", async () => {
  const fetcher = vi.fn<typeof fetch>();
  vi.stubGlobal("fetch", fetcher);
  render(
    <SetupPage
      status={{ initialized: false, target_configured: false, setup_token_required: false }}
      onComplete={vi.fn()}
    />,
  );
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("控制台账号", { exact: true }), "local-operator");
  await user.type(screen.getByLabelText("控制台密码", { exact: true }), "private-local-password");
  await user.type(screen.getByLabelText("确认控制台密码", { exact: true }), "different-password");
  const mode = screen.getByRole("checkbox", { name: "仅使用本地账号工作台（不配置线上管理目标）" });
  await user.click(mode);
  await user.click(screen.getByRole("button", { name: "完成初始化" }));
  expect(await screen.findByText("两次输入的密码不一致")).toBeVisible();
  expect(screen.getByLabelText("确认控制台密码", { exact: true })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(fetcher).not.toHaveBeenCalled();
  mode.focus();
  await user.keyboard(" ");
  expect(mode).not.toBeChecked();
  expect(screen.getByLabelText("Admin Base URL", { exact: true })).toBeVisible();
  expect(screen.getByLabelText("Admin Key", { exact: true })).toBeVisible();
});
