import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { ResourceDialog } from "../resource-dialog";
import { statusOptions, statusPage } from "./status-page-fixtures";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

it("调整组内监控和分组顺序后保存，稳定 ID、公开地址及原链接随监控保留", async () => {
  const submit = vi.fn();
  render(
    <ResourceDialog
      kind="status-pages"
      item={statusPage}
      options={statusOptions}
      pending={false}
      error={null}
      onClose={vi.fn()}
      onSubmit={submit}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "下移监控项 智谱" }));
  fireEvent.click(screen.getByRole("button", { name: "下移展示分组 1" }));
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        status_page: expect.objectContaining({
          groups: [
            { id: 12, name: "备用", monitorList: [] },
            {
              id: 11,
              name: "API",
              monitorList: [
                { id: 20, sendUrl: true },
                { id: 19, sendUrl: false, url: "https://example.com/one" },
              ],
            },
          ],
        }),
      }),
      expect.anything(),
    ),
  );
});

it("键盘可调整展示顺序，首项不能上移、末项不能下移", async () => {
  const user = userEvent.setup();
  render(
    <ResourceDialog
      kind="status-pages"
      item={statusPage}
      options={statusOptions}
      pending={false}
      error={null}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />,
  );
  expect(screen.getByRole("button", { name: "上移监控项 智谱" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "下移监控项 备用接口" })).toBeDisabled();
  await waitFor(() => expect(screen.getByLabelText("状态页标题")).toHaveFocus());
  screen.getByRole("button", { name: "下移监控项 智谱" }).focus();
  await user.keyboard("{Enter}");
  expect(screen.getByRole("button", { name: "下移监控项 智谱" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "上移监控项 备用接口" })).toBeDisabled();
});

it("保存中禁止排序、移除和修改公开地址", () => {
  render(
    <ResourceDialog
      kind="status-pages"
      item={statusPage}
      options={statusOptions}
      pending
      error={null}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />,
  );
  for (const button of screen.getAllByRole("button", { name: /上移|下移|移除/ }))
    expect(button).toBeDisabled();
  expect(screen.getByRole("checkbox", { name: /公开智谱的监控地址/ })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
});

it("可单独修改公开地址并移除展示项，保存只保留所选监控", async () => {
  const submit = vi.fn();
  render(
    <ResourceDialog
      kind="status-pages"
      item={statusPage}
      options={statusOptions}
      pending={false}
      error={null}
      onClose={vi.fn()}
      onSubmit={submit}
    />,
  );
  fireEvent.click(screen.getByRole("checkbox", { name: /公开智谱的监控地址/ }));
  fireEvent.click(screen.getByRole("button", { name: "移除监控项 备用接口" }));
  expect(screen.getByRole("button", { name: "上移监控项 智谱" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "下移监控项 智谱" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        status_page: expect.objectContaining({
          groups: expect.arrayContaining([
            {
              id: 11,
              name: "API",
              monitorList: [{ id: 19, sendUrl: true, url: "https://example.com/one" }],
            },
          ]),
        }),
      }),
      expect.anything(),
    ),
  );
});
