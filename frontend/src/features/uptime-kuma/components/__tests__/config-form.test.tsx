import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { ConfigForm as StandaloneConfigForm } from "../config-form";
import type { ComponentProps } from "react";
import { config } from "./fixtures";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

function ConfigForm(props: ComponentProps<typeof StandaloneConfigForm>) {
  return (
    <>
      <button form="kuma-config" type="submit" disabled={props.pending}>
        {props.pending ? "正在验证…" : "验证并保存"}
      </button>
      <StandaloneConfigForm {...props} />
    </>
  );
}

describe("Uptime Kuma 接入表单", () => {
  it("配置表单只包含字段，提交按钮可通过表单 ID 放在页头", () => {
    render(<ConfigForm config={config} pending={false} onSubmit={vi.fn()} />);
    for (const name of ["服务地址与指标", "管理账号"]) {
      expect(screen.getByRole("region", { name })).toHaveAttribute("data-slot", "card");
      expect(screen.getByRole("region", { name })).toHaveAttribute("data-size", "sm");
    }
    expect(screen.getByLabelText("服务地址").closest('[data-slot="card-content"]')).not.toBeNull();
    expect(screen.getByRole("button", { name: "验证并保存" }).closest("form")).toBeNull();
    expect(screen.getByRole("button", { name: "验证并保存" })).toHaveAttribute(
      "form",
      "kuma-config",
    );
  });
  it("键盘切换移除凭据时同步勾选状态并禁用管理字段", async () => {
    const user = userEvent.setup();
    render(<ConfigForm config={config} pending={false} onSubmit={vi.fn()} />);
    const checkbox = screen.getByRole("checkbox", { name: "移除管理凭据，仅查看指标" });
    expect(checkbox).toHaveAttribute("data-slot", "checkbox");
    checkbox.focus();
    await user.keyboard(" ");
    expect(checkbox).toBeChecked();
    expect(screen.getByLabelText("管理密码")).toBeDisabled();
  });
  it("编辑已配置服务时凭据不回显且留空可以提交", async () => {
    const submit = vi.fn();
    const user = userEvent.setup();
    render(<ConfigForm config={config} pending={false} onSubmit={submit} />);
    expect(screen.getByLabelText("API 密钥")).toHaveValue("");
    expect(screen.getByLabelText("管理密码")).toHaveValue("");
    await user.click(screen.getByRole("button", { name: "验证并保存" }));
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith(
        expect.objectContaining({ api_key: "", password: "", base_url: config.base_url }),
      ),
    );
  });
  it("更换地址但未输入密钥时显示字段错误并阻止提交", async () => {
    const submit = vi.fn();
    const user = userEvent.setup();
    render(<ConfigForm config={config} pending={false} onSubmit={submit} />);
    await user.clear(screen.getByLabelText("服务地址"));
    await user.type(screen.getByLabelText("服务地址"), "https://other.example");
    await user.click(screen.getByRole("button", { name: "验证并保存" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("请输入 API 密钥");
    expect(screen.getByLabelText("API 密钥")).toHaveAttribute("aria-invalid", "true");
    expect(submit).not.toHaveBeenCalled();
  });
  it("验证中禁止重复提交及修改凭据", async () => {
    const user = userEvent.setup();
    render(<ConfigForm config={config} pending onSubmit={vi.fn()} />);
    expect(screen.getByLabelText("服务地址")).toBeDisabled();
    expect(screen.getByRole("button", { name: "正在验证…" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "正在验证…" }));
    expect(screen.getByLabelText("管理密码")).toBeDisabled();
  });
  it("选择移除管理凭据时禁用账号密码并提交明确的移除标志", async () => {
    const submit = vi.fn();
    const user = userEvent.setup();
    render(<ConfigForm config={config} pending={false} onSubmit={submit} />);
    await user.click(screen.getByRole("checkbox", { name: "移除管理凭据，仅查看指标" }));
    expect(screen.getByLabelText("管理密码")).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "验证并保存" }));
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith(expect.objectContaining({ disable_management: true })),
    );
  });
});
