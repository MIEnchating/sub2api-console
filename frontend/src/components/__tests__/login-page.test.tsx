import { renderToStaticMarkup } from "react-dom/server";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { LoginPage } from "../../App";

describe("login page", () => {
  it("使用字段标签定位登录输入框并通过键盘提交时展示无效状态", async () => {
    const user = userEvent.setup();
    render(<LoginPage onLogin={() => undefined} />);
    const username = screen.getByRole("textbox", { name: "账号" });
    const password = screen.getByLabelText("密码", { exact: true });

    await user.click(screen.getByText("账号", { exact: true }));
    expect(username).toHaveFocus();
    await user.tab();
    expect(password).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "登录" })).toHaveFocus();
    await user.keyboard("{Enter}");

    expect(username).toHaveAttribute("aria-invalid", "true");
    expect(password).toHaveAttribute("aria-invalid", "true");
  });

  it("explains that the previous login expired", () => {
    const markup = renderToStaticMarkup(
      <LoginPage reason="登录已过期，请重新登录" onLogin={() => undefined} />,
    );

    expect(markup).toContain('role="alert"');
    expect(markup).toContain("登录已过期，请重新登录");
  });

  it("does not show an expiry warning for an ordinary logged-out visit", () => {
    const markup = renderToStaticMarkup(<LoginPage onLogin={() => undefined} />);

    expect(markup).not.toContain('role="alert"');
    expect(markup).not.toContain("登录已过期");
  });
});
