import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { account } from "../../__tests__/fixtures";
import { AccountIdentityCell } from "../account-pool-cells";

afterEach(cleanup);

const observedAt = "2026-10-11T12:00:00Z";

it("异常首字入口保持紧凑且键盘可打开详情并通过 Escape 返回入口", async () => {
  const user = userEvent.setup();
  render(<AccountIdentityCell account={{ ...account, probe_placeholder_at: observedAt }} />);
  const trigger = screen.getByRole("button", { name: "查看探活首字异常详情" });
  expect(trigger).toHaveClass("size-8");
  expect(trigger).not.toHaveTextContent("10/11");
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  trigger.focus();
  await user.keyboard("{Enter}");
  expect(await screen.findByRole("dialog", { name: "探活首字异常" })).toBeVisible();
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(trigger).toHaveFocus();
  expect(trigger).toHaveAttribute("aria-expanded", "false");
});

it("点击异常入口时分区展示原文与探活信息并支持关闭", async () => {
  const user = userEvent.setup();
  const preview = '" \t\u200b__-"';
  render(
    <AccountIdentityCell
      account={{
        ...account,
        probe_placeholder_at: observedAt,
        probe_placeholder: {
          group_name: "codex",
          request_model: "gpt-6-astra",
          result: "通过",
          attempt: 2,
          preview,
        },
      }}
    />,
  );
  await user.click(screen.getByRole("button", { name: "查看探活首字异常详情" }));
  const dialog = await screen.findByRole("dialog", { name: "探活首字异常" });
  const content = within(dialog);
  expect(content.getByLabelText("异常片段原文").textContent).toBe(preview);
  expect(dialog).toHaveTextContent("2026/10/11");
  expect(dialog).toHaveTextContent("codex");
  expect(dialog).toHaveTextContent("gpt-6-astra");
  expect(dialog).toHaveTextContent("第 2 次尝试");
  expect(dialog).toHaveTextContent("通过");
  expect(dialog).toHaveTextContent("不代表本次探活失败");
  await user.click(content.getByRole("button", { name: "关闭异常详情" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("异常原文与模型名称很长时详情受视口约束且内容可换行滚动", async () => {
  const user = userEvent.setup();
  const preview = `"${"_".repeat(80)}…"`;
  const model = "long-model-".repeat(30);
  render(
    <AccountIdentityCell
      account={{
        ...account,
        probe_placeholder_at: observedAt,
        probe_placeholder: { preview, request_model: model },
      }}
    />,
  );
  await user.click(screen.getByRole("button", { name: "查看探活首字异常详情" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog).toHaveClass(
    "max-w-[calc(100vw-2rem)]",
    "max-h-[var(--available-height)]",
    "overflow-y-auto",
  );
  expect(within(dialog).getByLabelText("异常片段原文")).toHaveClass(
    "break-all",
    "whitespace-pre-wrap",
  );
  expect(within(dialog).getByText(model)).toHaveClass("min-w-0", "break-all");
});

it("旧标记没有原文时展示缺失说明且不伪造原文区域", async () => {
  const user = userEvent.setup();
  render(<AccountIdentityCell account={{ ...account, probe_placeholder_at: observedAt }} />);
  await user.click(screen.getByRole("button", { name: "查看探活首字异常详情" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog).toHaveTextContent("未保存原文");
  expect(dialog).toHaveTextContent("这条历史记录仅保留了异常时间，无法还原当时的字符。");
  expect(within(dialog).queryByLabelText("异常片段原文")).not.toBeInTheDocument();
});

it("旧标记时间无效时详情显示时间未记录", async () => {
  const user = userEvent.setup();
  render(<AccountIdentityCell account={{ ...account, probe_placeholder_at: "invalid" }} />);
  await user.click(screen.getByRole("button", { name: "查看探活首字异常详情" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog).toHaveTextContent("时间未记录");
  expect(dialog).not.toHaveTextContent("Invalid Date");
});

it("未发现异常首字时不显示异常入口", () => {
  render(<AccountIdentityCell account={account} />);
  expect(screen.queryByRole("button", { name: "查看探活首字异常详情" })).not.toBeInTheDocument();
});

it("后续正常探活清除标记后刷新账号数据会移除入口及已打开详情", async () => {
  const user = userEvent.setup();
  const view = render(
    <AccountIdentityCell account={{ ...account, probe_placeholder_at: observedAt }} />,
  );
  await user.click(screen.getByRole("button", { name: "查看探活首字异常详情" }));
  expect(await screen.findByRole("dialog")).not.toHaveTextContent("占位字符已忽略");
  view.rerender(<AccountIdentityCell account={account} />);
  expect(screen.queryByRole("button", { name: "查看探活首字异常详情" })).not.toBeInTheDocument();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("异常入口与长账号名同行且有无异常时标题行高度一致", () => {
  const name = "超长账号名称".repeat(30);
  const view = render(
    <AccountIdentityCell account={{ ...account, name, probe_placeholder_at: observedAt }} />,
  );
  const heading = screen.getByText(name).closest('[data-slot="account-identity-heading"]');
  expect(heading).toHaveClass("h-8", "flex-nowrap");
  expect(heading).toContainElement(screen.getByRole("button", { name: "查看探活首字异常详情" }));
  expect(screen.getByText(name)).toHaveClass("min-w-0", "truncate");
  expect(screen.getByText(name)).not.toHaveClass("flex-1");
  view.rerender(<AccountIdentityCell account={{ ...account, name }} />);
  expect(screen.getByText(name).closest('[data-slot="account-identity-heading"]')).toHaveClass(
    "h-8",
  );
});
