import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { Task } from "@/api";
import { ProbeResultSlot } from "../account-probe-dialog";
import { PlatformProbeResultTable, platformProbeResults } from "../platform-probe-dialog";

afterEach(cleanup);

it("探活返回正文时展示完整内容并保留换行，不用成功摘要替代正文", () => {
  render(
    <ProbeResultSlot
      pending={false}
      error={null}
      result={{
        status: "passed",
        message: "上游已返回成功响应",
        response_text: "你好，\n连接正常。",
        request_model: "test",
        actual_model: "test",
        http_status: 200,
        latency_ms: 100,
      }}
    />,
  );
  const region = screen.getByRole("region", { name: "探活响应内容" });
  expect(region.textContent).toContain("你好，\n连接正常。");
  expect(within(region).queryByText("上游已返回成功响应")).not.toBeInTheDocument();
});

it("探活没有可提取正文但返回 JSON 时提供实际响应原文", async () => {
  render(
    <ProbeResultSlot
      pending={false}
      error={null}
      result={{
        status: "passed",
        message: "上游已返回成功响应",
        response_json: '{"id":"reply-42","model":"test"}',
        request_model: "test",
        actual_model: "test",
        http_status: 200,
        latency_ms: 100,
      }}
    />,
  );
  expect(await screen.findByRole("textbox", { name: "上游响应原文" })).toHaveTextContent(
    "reply-42",
  );
});

it("批量探活成功时在结果表展示该账号的实际回复", () => {
  const task = {
    result: {
      results: [
        {
          account_id: "41",
          account_name: "账号甲",
          result: "通过",
          response_text: "已收到你的请求。",
          request_model: "test",
          status_code: 200,
        },
      ],
    },
  } as unknown as Task;
  render(<PlatformProbeResultTable results={platformProbeResults(task)} />);
  expect(screen.getByRole("columnheader", { name: "响应内容" })).toBeVisible();
  expect(screen.getByText("已收到你的请求。")).toBeVisible();
});
