// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { bench, describe } from "vitest";
import type { UpstreamAccountRequestUsage } from "@/api";
import { UpstreamUsageSummary } from "../upstream-usage-summary";

for (const count of [1000, 10000]) {
  describe(`${count} 个账号的使用明细刷新`, () => {
    const accounts: UpstreamAccountRequestUsage[] = Array.from({ length: count }, (_, index) => ({
      account_id: String(index + 1),
      account_name: `账号 ${index + 1}`,
      request_count: index + 1,
      share: 100 / count,
    }));
    const snapshots = [accounts, accounts.map((account) => ({ ...account }))];
    let revision = 0;
    let view: ReturnType<typeof render>;
    function summary() {
      return (
        <UpstreamUsageSummary
          name="隔离基准"
          requestCount={(count * (count + 1)) / 2}
          requestWindowDays={30}
          accountRequestUsage={snapshots[revision % 2]}
        />
      );
    }
    bench(
      "刷新统计保留当前页",
      () => {
        revision += 1;
        view.rerender(summary());
      },
      {
        time: 1500,
        warmupTime: 500,
        iterations: 20,
        setup: () => {
          view = render(summary());
          fireEvent.click(screen.getByRole("button", { name: "查看隔离基准的账号使用率" }));
          if (screen.getAllByRole("row").length !== 21) throw new Error("首次分页应保留 20 个账号");
        },
        teardown: cleanup,
      },
    );
  });
}
