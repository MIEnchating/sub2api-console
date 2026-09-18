import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { AccountRecentResults } from "@/components/account-recent-results";

it.each([
  ['HTTP 429：{"error":{"code":"rate_limit_exceeded","message":"Too many requests"}}', "限流"],
  ['HTTP 429：{"error":{"code":"insufficient_balance","message":"余额不足"}}', "余额不足"],
  ['{"error":{"type":"rate_limit_error"}}', "限流"],
  ["HTTP 429（上游未返回错误详情）", "上游请求失败"],
  [
    "Upstream rejected this request. " + "detail ".repeat(30) + "Final reason: policy Q17.",
    "上游请求失败",
  ],
  ["Quota policy Q17 rejected this request", "上游请求失败"],
  ["Rate limit or insufficient balance", "上游请求失败"],
])("错误为 %s 时只显示确定原因并保留上游详情", async (reason, label) => {
  render(
    <AccountRecentResults
      results={[
        {
          result: "失败",
          event_type: "rate_limited_or_exhausted",
          score: 15,
          observed_at: null,
          latency_ms: null,
          failure_reason: reason,
          source: "active-probe",
        },
      ]}
    />,
  );
  await userEvent.tab();
  const tip = await screen.findByRole("tooltip");
  expect(tip).toHaveTextContent(`${label} · 15 分`);
  expect(tip).toHaveTextContent(reason);
  expect(tip).not.toHaveTextContent("限流或额度不足");
});

it("历史记录只保存混合分类时明确详情缺失，不推测具体原因", async () => {
  render(
    <AccountRecentResults
      results={[
        {
          result: "失败",
          event_type: "rate_limited_or_exhausted",
          score: 15,
          observed_at: null,
          latency_ms: null,
          failure_reason: "上游限流或额度不足",
          source: "active-probe",
        },
      ]}
    />,
  );
  await userEvent.tab();
  const tip = await screen.findByRole("tooltip");
  expect(tip).toHaveTextContent("历史记录未保存具体错误");
  expect(tip).not.toHaveTextContent("限流或额度不足");
});

it("上游用网关状态返回明确余额错误时按具体原因展示", async () => {
  render(
    <AccountRecentResults
      results={[
        {
          result: "失败",
          event_type: "gateway_error",
          score: 25,
          observed_at: null,
          latency_ms: null,
          failure_reason: "HTTP 502：余额不足",
          source: "active-probe",
        },
      ]}
    />,
  );
  await userEvent.tab();
  const tip = await screen.findByRole("tooltip");
  expect(tip).toHaveTextContent("余额不足 · 25 分");
  expect(tip).not.toHaveTextContent("网关错误");
});
