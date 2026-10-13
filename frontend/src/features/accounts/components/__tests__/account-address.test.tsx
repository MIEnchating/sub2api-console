import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { account } from "../../__tests__/fixtures";
import { AccountIdentityCell } from "../account-pool-cells";

it("上游改址但绑定 Host 保留时，账号展示当前地址和端口，键盘提示保留完整路径", async () => {
  const user = userEvent.setup();
  render(
    <AccountIdentityCell
      account={{
        ...account,
        upstream_base_url: "http://192.0.2.12:8080/admin",
        base_url: "https://models.example.test/v1",
      }}
    />,
  );

  const address = screen.getByText("192.0.2.12:8080");
  expect(address).toHaveClass("truncate");
  expect(screen.queryByText(account.upstream_host!)).not.toBeInTheDocument();
  address.focus();
  expect(await screen.findByRole("tooltip")).toHaveTextContent("http://192.0.2.12:8080/admin");
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
});

it("刷新账号取得新上游地址时，不再保留上一次显示的域名", () => {
  const view = render(
    <AccountIdentityCell account={{ ...account, upstream_base_url: "https://old.example.test" }} />,
  );
  expect(screen.getByText("old.example.test")).toBeVisible();
  view.rerender(
    <AccountIdentityCell account={{ ...account, upstream_base_url: "https://new.example.test" }} />,
  );
  expect(screen.getByText("new.example.test")).toBeVisible();
  expect(screen.queryByText("old.example.test")).not.toBeInTheDocument();
});

it.each([null, "", "invalid-url", "javascript:alert(1)"])(
  "上游访问地址为 %s 时，仍可查看已记录的绑定 Host",
  async (baseURL) => {
    render(<AccountIdentityCell account={{ ...account, upstream_base_url: baseURL }} />);
    screen.getByText(account.upstream_host!).focus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(account.upstream_host!);
  },
);

it("上游访问地址和绑定 Host 均缺失时保留缺失提示", () => {
  render(
    <AccountIdentityCell account={{ ...account, upstream_base_url: null, upstream_host: null }} />,
  );
  expect(screen.getByText("Host 未记录")).toBeVisible();
});
