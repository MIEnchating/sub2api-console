import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";
import { LoginPage } from "../login-page";

function renderLogin(element: ReactElement): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { gcTime: 0 } },
  });
  render(<QueryClientProvider client={client}>{element}</QueryClientProvider>);
}

describe("登录表单", () => {
  it("使用标签和键盘访问所有登录控件，空值提交后标记无效字段", async () => {
    const user = userEvent.setup();
    renderLogin(<LoginPage onLogin={() => undefined} />);
    const username = screen.getByRole("textbox", { name: "账号" });
    const password = screen.getByLabelText("密码", { exact: true });

    await user.click(screen.getByText("账号", { exact: true, selector: "label" }));
    expect(username).toHaveFocus();
    await user.tab();
    expect(password).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "显示密码" })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "登录" })).toHaveFocus();
    await user.keyboard("{Enter}");

    expect(username).toHaveAttribute("aria-invalid", "true");
    expect(password).toHaveAttribute("aria-invalid", "true");
    expect(username).toHaveFocus();
  });

  it("收到会话过期原因时显示提示并保留可填写的表单", () => {
    renderLogin(<LoginPage reason="登录已过期，请重新登录" onLogin={() => undefined} />);
    expect(screen.getByRole("alert")).toHaveTextContent("登录已过期，请重新登录");
    expect(screen.getByRole("textbox", { name: "账号" })).toBeEnabled();
  });

  it("普通未登录访问不显示会话过期提示", () => {
    renderLogin(<LoginPage onLogin={() => undefined} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
