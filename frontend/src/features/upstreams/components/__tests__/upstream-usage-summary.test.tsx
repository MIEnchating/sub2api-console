import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { UpstreamUsageSummary } from "../upstream-usage-summary";

it("默认保留紧凑汇总，键盘打开账号明细时不在上游行内展开", async () => {
  const user = userEvent.setup();
  const view = render(
    <UpstreamUsageSummary
      name="示例上游"
      spending={{ total: "12.340000", started_at: "2026-10-01", updated_at: "2026-10-09" }}
      requestCount={20}
      requestWindowDays={30}
      accountRequestUsage={[
        { account_id: "a", account_name: "账号 A", request_count: 4, share: 20 },
        { account_id: "b", account_name: "账号 B", request_count: 3, share: 15 },
      ]}
    />,
  );
  expect(screen.getByText("12.34")).toBeInTheDocument();
  expect(screen.getByText("20 次")).toBeInTheDocument();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  const trigger = screen.getByRole("button", { name: "查看示例上游的账号使用率" });
  trigger.focus();
  await user.keyboard("{Enter}");
  const dialog = await screen.findByRole("dialog", { name: "示例上游 · 使用统计" });
  expect(within(dialog).getByRole("row", { name: /账号 A.*4.*20.00%/ })).toBeVisible();
  expect(within(dialog).getByRole("row", { name: /账号 B.*3.*15.00%/ })).toBeVisible();
  expect(within(view.container).queryByText("账号 A")).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  expect(trigger).toHaveFocus();
});

it("余额未同步时显示待同步，无账号时明细提供空状态", async () => {
  const user = userEvent.setup();
  render(
    <UpstreamUsageSummary
      name="空上游"
      spending={null}
      requestCount={0}
      requestWindowDays={30}
      accountRequestUsage={[]}
    />,
  );
  expect(screen.getByText("待同步")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "查看空上游的账号使用率" }));
  expect(await screen.findByText("暂无绑定账号")).toBeVisible();
});

it("统计在左、详情按钮在右，键盘依次聚焦消费、请求数和详情", async () => {
  const user = userEvent.setup();
  render(
    <UpstreamUsageSummary
      name="示例上游"
      spending={{ total: "12.340000", started_at: "2026-10-01", updated_at: "2026-10-09" }}
      requestCount={20}
      requestWindowDays={30}
      accountRequestUsage={[]}
    />,
  );
  await user.tab();
  expect(screen.getByText("12.34")).toHaveFocus();
  expect(await screen.findByText("12.340000")).toBeVisible();
  await user.tab();
  expect(screen.getByText("20 次")).toHaveFocus();
  expect(await screen.findByText("近 30 天请求数")).toBeVisible();
  await user.tab();
  expect(screen.getByRole("button", { name: "查看示例上游的账号使用率" })).toHaveFocus();
});

it("打开统计明细后可用 Tab 查看原始消费和被截断的账号全名", async () => {
  const user = userEvent.setup();
  const name = "生产账号的完整名称".repeat(8);
  render(
    <UpstreamUsageSummary
      name="键盘上游"
      spending={{ total: "12.123456789", started_at: "2026-10-01", updated_at: "2026-10-11" }}
      requestCount={1234}
      requestWindowDays={30}
      accountRequestUsage={[
        { account_id: "stable-account", account_name: name, request_count: 1234, share: 100 },
      ]}
    />,
  );
  await user.click(screen.getByRole("button", { name: "查看键盘上游的账号使用率" }));
  const dialog = await screen.findByRole("dialog", { name: "键盘上游 · 使用统计" });
  await user.tab();
  expect(within(dialog).getByText("12.123456")).toHaveFocus();
  expect(await screen.findByText("12.123456789")).toBeVisible();
  await user.tab();
  expect(within(dialog).getByText(name)).toHaveFocus();
  expect(await screen.findByText(`${name} · ID stable-account`)).toBeVisible();
});
