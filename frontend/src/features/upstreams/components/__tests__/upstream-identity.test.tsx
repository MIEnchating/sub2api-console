import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { UpstreamIdentity, upstreamIdentityLayout } from "../upstream-identity";

describe("上游列表地址", () => {
  it("上游地址与旧 Host 不同时显示当前地址并提供一致的链接名称和跳转目标", () => {
    render(
      <UpstreamIdentity
        name="测试上游"
        host="old.example.test"
        hosts={["old.example.test"]}
        baseUrl="https://new.example.test"
      />,
    );

    const link = screen.getByRole("link", { name: "访问 测试上游（new.example.test）" });
    expect(link).toHaveTextContent("new.example.test");
    expect(link).not.toHaveTextContent("old.example.test");
    expect(link).toHaveAttribute("href", "https://new.example.test");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noreferrer");
  });

  it("列表收到新上游地址而 Host 不变时更新链接文字和目标", () => {
    const view = render(
      <UpstreamIdentity
        name="测试上游"
        host="old.example.test"
        hosts={["old.example.test"]}
        baseUrl="https://old.example.test"
      />,
    );
    expect(screen.getByRole("link")).toHaveTextContent("old.example.test");

    view.rerender(
      <UpstreamIdentity
        name="测试上游"
        host="old.example.test"
        hosts={["old.example.test"]}
        baseUrl="https://new.example.test/admin"
      />,
    );

    const link = screen.getByRole("link", { name: "访问 测试上游（new.example.test）" });
    expect(link).toHaveTextContent("new.example.test");
    expect(link).toHaveAttribute("href", "https://new.example.test/admin");
  });

  it.each([
    ["http://new.example.test:8080/admin", "new.example.test:8080"],
    ["https://[::1]:8443/admin", "[::1]:8443"],
  ])("上游地址 %s 含端口和路径时保留完整跳转地址", (baseUrl, host) => {
    render(
      <UpstreamIdentity name="测试上游" host="old.example.test" hosts={[]} baseUrl={baseUrl} />,
    );

    const link = screen.getByRole("link", { name: `访问 测试上游（${host}）` });
    expect(link).toHaveTextContent(host);
    expect(link).toHaveAttribute("href", baseUrl);
  });

  it.each(["", "invalid-address", "javascript:alert(1)"])(
    "上游地址 %s 不可用时回退到 Host 的 HTTPS 链接",
    (baseUrl) => {
      render(
        <UpstreamIdentity
          name="备用上游"
          host="backup.example.test"
          hosts={[]}
          baseUrl={baseUrl}
        />,
      );

      const link = screen.getByRole("link", { name: "访问 备用上游（backup.example.test）" });
      expect(link).toHaveTextContent("backup.example.test");
      expect(link).toHaveAttribute("href", "https://backup.example.test");
    },
  );

  it("超长名称和地址保持单行截断且完整地址仍可访问", () => {
    const name = "超长上游名称".repeat(20);
    const host = `${"long-subdomain.".repeat(8)}example.test`;
    render(
      <UpstreamIdentity
        name={name}
        host="old.example.test"
        hosts={[]}
        baseUrl={`https://${host}`}
      />,
    );

    expect(screen.getByText(name)).toHaveClass("truncate");
    const link = screen.getByRole("link", { name: `访问 ${name}（${host}）` });
    expect(link).toHaveClass("min-w-0");
    expect(within(link).getByText(host)).toHaveClass("truncate");
    expect(link).toHaveAttribute("href", `https://${host}`);
    expect(upstreamIdentityLayout.root).toContain("min-w-0");
  });
});
