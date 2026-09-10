import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MonitorWorkspace } from "../monitor-workspace";
import { MonitorDialog } from "../monitor-dialog";
import { monitor } from "./fixtures";

const group = { ...monitor, id: 7, key: "id:7", name: "openai", type: "group" };
const child = { ...monitor, id: 19, key: "id:19", name: "codex", parent: 7 };

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

describe("监控分组", () => {
  it("父组位于子项之后返回时，仍按层级展示且折叠仅隐藏该组子项", async () => {
    const user = userEvent.setup();
    render(
      <MonitorWorkspace
        monitors={[child, { ...monitor, id: 20, key: "id:20" }, group]}
        management
        disabled={false}
        onEdit={vi.fn()}
        onAction={vi.fn()}
      />,
    );
    const rows = screen.getAllByRole("row");
    const parentRow = screen.getByRole("row", { name: /^openai 未分组/ });
    const childRow = screen.getByRole("row", { name: /^codex openai/ });
    expect(rows.indexOf(parentRow)).toBeLessThan(rows.indexOf(childRow));
    expect(within(childRow).getByText("openai", { exact: true })).toBeVisible();
    const toggle = within(parentRow).getByRole("button", { name: "收起分组 openai" });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    await user.click(toggle);
    expect(screen.queryByRole("button", { name: "查看 codex 详情" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "查看 智谱 详情" })).toBeVisible();
    const expand = screen.getByRole("button", { name: "展开分组 openai" });
    expand.focus();
    await user.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "查看 codex 详情" })).toBeVisible();
  });

  it("收起分组后搜索子项，会展开匹配的父组并保留层级", async () => {
    const user = userEvent.setup();
    render(
      <MonitorWorkspace
        monitors={[group, child]}
        management
        disabled={false}
        onEdit={vi.fn()}
        onAction={vi.fn()}
      />,
    );
    await user.click(screen.getByRole("button", { name: "收起分组 openai" }));
    await user.type(screen.getByRole("textbox", { name: "搜索监控项或地址" }), "codex");
    expect(screen.getByRole("button", { name: "查看 openai 详情" })).toBeVisible();
    expect(screen.getByRole("button", { name: "查看 codex 详情" })).toBeVisible();
  });

  it("编辑子项未更改所属分组时提交原有稳定分组 ID", async () => {
    const submit = vi.fn();
    const user = userEvent.setup();
    render(
      <MonitorDialog
        monitor={child}
        monitors={[group, child]}
        pending={false}
        onClose={vi.fn()}
        onSubmit={submit}
      />,
    );
    expect(screen.getByRole("combobox", { name: "所属分组" })).toHaveTextContent("openai");
    await user.click(screen.getByRole("button", { name: "保存监控项" }));
    expect(submit).toHaveBeenCalledWith(expect.objectContaining({ parent: 7 }));
  });
});
