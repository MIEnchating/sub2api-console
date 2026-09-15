import type { ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkbenchTemplate } from "@/api";
import { WorkbenchImport } from "../components/workbench-import";
import { WorkbenchOAuthOptions } from "../components/workbench-oauth-options";
import { WorkbenchTemplates } from "../components/workbench-templates";
import { defaultConfig, workbenchKeys } from "../constants";

let client: QueryClient;
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function templates(preferred: string): WorkbenchTemplate[] {
  return ["team", "personal"].map((id) => ({
    id,
    revision: 2,
    name: id === "team" ? "团队模板" : "个人模板",
    preferred: id === preferred,
    priority: 0,
    match: { plan_type: "", email_domain: "" },
    config: defaultConfig,
    target_url: "https://sub2api.example.test",
  }));
}

function mount(component: ReactElement): void {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(<QueryClientProvider client={client}>{component}</QueryClientProvider>);
}

describe("首选模板", () => {
  it.each(["账号导入", "授权导入"])(
    "%s首次载入选择首选，用户改为自动匹配后刷新不会覆盖选择",
    async (kind) => {
      vi.stubGlobal(
        "fetch",
        vi.fn<typeof fetch>(async () => Response.json(templates("team"))),
      );
      mount(
        kind === "账号导入" ? (
          <WorkbenchImport />
        ) : (
          <WorkbenchOAuthOptions disabled={false} onChange={vi.fn()} onSubmit={vi.fn()} />
        ),
      );
      const user = userEvent.setup();
      await waitFor(() =>
        expect(screen.getByRole("combobox", { name: "配置模板" })).toHaveTextContent("团队模板"),
      );
      await user.click(screen.getByRole("combobox", { name: "配置模板" }));
      await user.click(await screen.findByRole("option", { name: "自动匹配模板" }));
      await act(async () => {
        client.setQueryData(workbenchKeys.templates, templates("personal"));
      });
      expect(screen.getByRole("combobox", { name: "配置模板" })).toHaveTextContent("自动匹配模板");
    },
  );

  it("授权导入初次提交使用首选模板稳定ID", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async () => Response.json(templates("team"))),
    );
    const submit = vi.fn();
    mount(<WorkbenchOAuthOptions disabled={false} onChange={vi.fn()} onSubmit={submit} />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "预览授权账号" }));
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith(
        expect.objectContaining({ template_id: "team" }),
        expect.anything(),
      ),
    );
  });

  it("键盘切换首选使用当前模板ID和版本，刷新后可取消首选", async () => {
    let preferred = "team";
    const writes: Array<{ path: string; body: unknown }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (input, init) => {
        const path = String(input);
        if (init?.method === "PUT") {
          const body = JSON.parse(String(init.body)) as { preferred: boolean };
          writes.push({ path, body });
          preferred = body.preferred ? "personal" : "";
          return Response.json(templates(preferred)[1]);
        }
        return Response.json(templates(preferred));
      }),
    );
    mount(<WorkbenchTemplates />);
    const user = userEvent.setup();
    const action = await screen.findByRole("button", { name: "设为首选模板：个人模板" });
    action.focus();
    await user.keyboard("{Enter}");
    const selected = await screen.findByRole("button", { name: "取消首选模板：个人模板" });
    expect(selected).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "设为首选模板：团队模板" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    expect(writes).toEqual([
      {
        path: "/api/account-workbench/templates/personal/preference",
        body: { revision: 2, preferred: true },
      },
    ]);
    await user.click(selected);
    await screen.findByRole("button", { name: "设为首选模板：个人模板" });
    expect(writes[1]?.body).toEqual({ revision: 2, preferred: false });
  });

  it("更新首选失败后保留原首选状态并恢复操作", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (_input, init) => {
        if (init?.method === "PUT")
          return Response.json({ detail: "模板版本已变更，请刷新" }, { status: 409 });
        return Response.json(templates("team"));
      }),
    );
    mount(<WorkbenchTemplates />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "设为首选模板：个人模板" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "设为首选模板：个人模板" })).toBeEnabled(),
    );
    expect(screen.getByRole("button", { name: "取消首选模板：团队模板" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });
});
