import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { ConfigForm } from "../config-form";
import { config } from "./fixtures";

afterEach(() => vi.unstubAllGlobals());

function mount(): ReturnType<typeof vi.fn> {
  const submit = vi.fn();
  render(
    <>
      <ConfigForm config={config} pending={false} onSubmit={submit} />
      <button form="kuma-config" type="submit">
        验证并保存
      </button>
    </>,
  );
  return submit;
}

it("更换服务地址并填写新密钥后仍须补充管理密码，缺失时阻止提交", async () => {
  const submit = mount();
  const user = userEvent.setup();
  await user.clear(screen.getByLabelText("服务地址"));
  await user.type(screen.getByLabelText("服务地址"), "https://other.example");
  await user.type(screen.getByLabelText("API 密钥"), "test-key");
  const password = screen.getByLabelText("管理密码");
  expect(password).toHaveAttribute("aria-required", "true");
  await user.click(screen.getByRole("button", { name: "验证并保存" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("请输入管理账号的密码");
  expect(password).toHaveAttribute("aria-invalid", "true");
  expect(submit).not.toHaveBeenCalled();

  await user.type(password, "new-test-password");
  await user.click(screen.getByRole("button", { name: "验证并保存" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        base_url: "https://other.example",
        password: "new-test-password",
      }),
    ),
  );
});

it("原地址仅补充 dashboard 路径时允许留空保留原凭据", async () => {
  const submit = mount();
  const user = userEvent.setup();
  await user.clear(screen.getByLabelText("服务地址"));
  await user.type(screen.getByLabelText("服务地址"), `${config.base_url}/dashboard/`);
  expect(screen.getByLabelText("管理密码")).toHaveAttribute("aria-required", "false");
  await user.click(screen.getByRole("button", { name: "验证并保存" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        api_key: "",
        password: "",
      }),
    ),
  );
});

it("更换地址并明确移除管理凭据时只需新指标密钥，无需管理密码", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const submit = mount();
  const user = userEvent.setup();
  await user.clear(screen.getByLabelText("服务地址"));
  await user.type(screen.getByLabelText("服务地址"), "https://other.example");
  await user.type(screen.getByLabelText("API 密钥"), "test-key");
  await user.click(screen.getByRole("checkbox", { name: "移除管理凭据，仅查看指标" }));
  expect(screen.getByLabelText("管理密码")).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "验证并保存" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        disable_management: true,
        password: "",
      }),
    ),
  );
});
