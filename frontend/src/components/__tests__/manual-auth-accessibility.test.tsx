import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ManualAuthForm, ManualAuthHeadersEditor } from "../../App";

const restoreSelectorMatching = vi.hoisted(() => {
  const matches = Element.prototype.matches;
  // NWSAPI captures this method during App imports, before beforeEach runs.
  Element.prototype.matches = function (selector: string): boolean {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  };
  return () => {
    Element.prototype.matches = matches;
  };
});
afterAll(restoreSelectorMatching);

const clients: QueryClient[] = [];

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  // JSDOM 26 recurses on top-layer and native select picker selectors.
  const getComputedStyle = window.getComputedStyle;
  vi.spyOn(window, "getComputedStyle").mockImplementation((element, pseudoElement) => {
    if (element instanceof HTMLSelectElement) {
      const style = document.createElement("div").style;
      style.display = "none";
      return style;
    }
    return getComputedStyle(element, pseudoElement);
  });
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderForm(upstreamType = "sub2api"): void {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  });
  clients.push(client);
  client.setQueryData(["auth-recovery-config"], { auth_records: [], vault_entries: [] });
  render(
    <QueryClientProvider client={client}>
      <ManualAuthForm host="api.example.test" upstreamType={upstreamType} />
    </QueryClientProvider>,
  );
}

describe("手动鉴权表单可访问性", () => {
  it.each([
    { platform: "sub2api", labels: ["Token", "刷新 Token"] },
    { platform: "newapi", labels: ["Admin Key", "User ID"] },
    { platform: "custom", labels: ["Token"] },
  ])("$platform 默认鉴权模式下凭据字段关联可见标签", (fixture) => {
    renderForm(fixture.platform);

    for (const label of fixture.labels) {
      expect(screen.getByLabelText(label, { exact: true })).toBeVisible();
    }
  });

  it("鉴权方式选择器提供名称并支持键盘打开选项", async () => {
    renderForm();

    const mode = screen.getByRole("combobox", { name: "鉴权方式" });
    mode.focus();
    fireEvent.keyDown(mode, { key: "ArrowDown" });

    expect(mode).toHaveAttribute("aria-expanded", "true");
    await waitFor(() => expect(screen.getByRole("option", { name: "密码箱登录" })).toBeVisible());
  });

  it("切换密码箱登录时密码项选择器关联标签", async () => {
    const user = userEvent.setup();
    renderForm();
    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByRole("option", { name: "密码箱登录" }));

    expect(screen.getByRole("combobox", { name: "密码箱密码项" })).toBeVisible();
  });

  it("切换账号密码并启用保存时用户名密码及凭据名称关联标签", async () => {
    const user = userEvent.setup();
    renderForm();
    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByRole("option", { name: "自定义账号密码" }));
    fireEvent.click(screen.getByRole("switch", { name: /登录成功后/ }));

    for (const label of ["用户名", "密码", "凭据名称（可选）"]) {
      expect(screen.getByLabelText(label, { exact: true })).toBeVisible();
    }
  });

  it("Headers 编辑器提供可访问名称", () => {
    render(<ManualAuthHeadersEditor value="" onChange={() => undefined} />);

    expect(screen.getByRole("textbox", { name: "Headers JSON" })).toBeVisible();
  });

  it("提交无效 Headers 时文本框标记无效并关联错误说明，重新输入时清除", () => {
    renderForm();
    fireEvent.click(screen.getByRole("switch", { name: /自定义 Headers/ }));
    const headers = screen.getByRole("textbox");
    fireEvent.change(headers, { target: { value: "{" } });
    fireEvent.click(screen.getByRole("button", { name: "验证并保存" }));

    const error = screen.getByRole("alert");
    expect(headers).toHaveAttribute("aria-invalid", "true");
    expect(headers).toHaveAccessibleDescription(error.textContent ?? "");

    fireEvent.change(headers, { target: { value: '{"X-Test":"valid"}' } });

    expect(headers).toHaveAttribute("aria-invalid", "false");
    expect(headers).not.toHaveAccessibleDescription();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
