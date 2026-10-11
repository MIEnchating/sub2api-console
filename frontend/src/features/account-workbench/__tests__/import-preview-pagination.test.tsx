import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { ImportPreview } from "../components/import-preview";
import { importPreviewFixture } from "./import-preview-fixture";

afterEach(cleanup);

it("500 项账号预览每页挂载 20 行，键盘翻页保持原序号且确认整个批次", async () => {
  const confirm = vi.fn();
  render(
    <ImportPreview
      preview={importPreviewFixture(500)}
      pending={false}
      onClose={vi.fn()}
      onConfirm={confirm}
    />,
  );
  const table = screen.getByRole("table", { name: "本批账号预览" });
  expect(within(table).getAllByRole("row")).toHaveLength(21);
  expect(screen.queryByText("account-21@example.test")).not.toBeInTheDocument();
  expect(screen.getByText(/共 500 项/)).toBeVisible();
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();

  const user = userEvent.setup();
  await user.tab();
  screen.getByRole("button", { name: "转到下一页" }).focus();
  await user.keyboard("{Enter}");
  const row = screen.getByRole("row", { name: /account-21@example.test/ });
  expect(within(row).getByRole("cell", { name: "21" })).toBeVisible();
  expect(screen.queryByText("account-1@example.test")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "确认并开始" }));
  expect(confirm).toHaveBeenCalledOnce();
});

it("末页和每页行数选择均可查看完整批次，切换行数后回到首页", async () => {
  render(
    <ImportPreview
      preview={importPreviewFixture(41)}
      pending={false}
      onClose={vi.fn()}
      onConfirm={vi.fn()}
    />,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "转到最后一页" }));
  expect(screen.getByText("account-41@example.test")).toBeVisible();
  expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(2);
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  await user.click(screen.getByRole("combobox", { name: "每页行数" }));
  await user.click(await screen.findByRole("option", { name: "50" }));
  expect(screen.getByText("account-1@example.test")).toBeVisible();
  expect(screen.getByText("account-41@example.test")).toBeVisible();
  expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(42);
});

it("同一预览刷新保留页码，数量缩减收敛到有效页，新预览版本重置首页", async () => {
  const props = { pending: false, onClose: vi.fn(), onConfirm: vi.fn() };
  const preview = importPreviewFixture(41);
  const view = render(<ImportPreview {...props} preview={preview} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  view.rerender(<ImportPreview {...props} preview={{ ...preview }} />);
  expect(screen.getByText("account-21@example.test")).toBeVisible();
  view.rerender(<ImportPreview {...props} preview={importPreviewFixture(1)} />);
  expect(screen.getByText("account-1@example.test")).toBeVisible();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  view.rerender(<ImportPreview {...props} preview={preview} />);
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  view.rerender(<ImportPreview {...props} preview={{ ...preview, revision: 2 }} />);
  expect(screen.getByText("account-1@example.test")).toBeVisible();
  expect(screen.queryByText("account-21@example.test")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  view.rerender(<ImportPreview {...props} preview={{ ...preview, id: "preview-two" }} />);
  expect(screen.getByText("account-1@example.test")).toBeVisible();
});

it("非当前页存在错误仍展示具体原因并禁止启动整个批次", async () => {
  const preview = importPreviewFixture(41);
  preview.errors = [{ index: 40, message: "账号资料缺少官方工作区" }];
  const confirm = vi.fn();
  render(<ImportPreview preview={preview} pending={false} onClose={vi.fn()} onConfirm={confirm} />);
  expect(screen.getByText("第 41 项：账号资料缺少官方工作区")).toBeVisible();
  expect(screen.queryByText("account-41@example.test")).not.toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "确认并开始" }));
  expect(confirm).not.toHaveBeenCalled();
});

it("空批次禁止确认且分页边界禁用，返回修改仍可操作", async () => {
  const close = vi.fn();
  render(
    <ImportPreview
      preview={importPreviewFixture(0)}
      pending={false}
      onClose={close}
      onConfirm={vi.fn()}
    />,
  );
  expect(screen.getByRole("button", { name: "确认并开始" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  await userEvent.setup().click(screen.getByRole("button", { name: "返回修改" }));
  expect(close).toHaveBeenCalledOnce();
});

it("启动请求等待期间分页保持可读，重复确认和返回修改禁用且不显示伪造进度", async () => {
  const confirm = vi.fn();
  const close = vi.fn();
  render(
    <ImportPreview
      preview={importPreviewFixture(21)}
      pending
      onClose={close}
      onConfirm={confirm}
    />,
  );
  expect(screen.getByRole("status", { name: "正在创建账号处理任务" })).toBeVisible();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByText("account-21@example.test")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认并开始" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "返回修改" })).toBeDisabled();
  expect(confirm).not.toHaveBeenCalled();
  expect(close).not.toHaveBeenCalled();
});
