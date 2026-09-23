import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { AnimationResult } from "@/api";
import { PrecheckAccountResult } from "../precheck-account-result";

function result(): AnimationResult {
  return {
    account_id: "41",
    account_name: "检测账号",
    mode: "precheck",
    model: "gpt-6-astra",
    status: "succeeded",
    request_id: "precheck-41",
    duration_ms: 1500,
    completed_at: "2026-09-15T00:00:00Z",
    precheck: {
      verdict: "passed",
      profile_version: "astra-v1",
      questions: [
        { id: "candy", verdict: "passed", answer: "21", request_id: "candy-41" },
        {
          id: "knowledge-cutoff",
          verdict: "passed",
          answer: "无法提供知识截至日期。".repeat(80),
          request_id: "cutoff-41",
        },
      ],
    },
  };
}

it("前置检测等待结果时提供可访问忙碌状态，不伪造确定进度", () => {
  render(<PrecheckAccountResult activity={{ mode: "precheck", status: "running" }} />);
  expect(screen.getByRole("status", { name: "正在执行前置检测" })).toBeVisible();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(screen.getByRole("region", { name: "前置检测结果" })).toHaveClass("h-auto", "shrink-0");
});

it("前置检测取消且没有返回结果时明确展示取消状态", () => {
  render(<PrecheckAccountResult status="cancelled" />);
  expect(screen.getByRole("region", { name: "前置检测结果" })).toHaveTextContent("已取消");
  expect(screen.queryByText("未检测")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "查看前置检测详情" })).not.toBeInTheDocument();
});

it("尚未检测时仅按状态行高度展示且不提供空详情", () => {
  render(<PrecheckAccountResult />);
  const summary = screen.getByRole("region", { name: "前置检测结果" });
  expect(summary).toHaveClass("h-auto", "shrink-0");
  expect(summary).toHaveTextContent("未检测");
  expect(screen.queryByRole("button", { name: "查看前置检测详情" })).not.toBeInTheDocument();
});

it("糖果题回答非 21 时卡片仅在前置检测标题旁展示一次降智结论", () => {
  const fixture = result();
  fixture.precheck = {
    verdict: "not_passed",
    profile_version: "astra-v1",
    questions: [{ id: "candy", verdict: "not_passed", answer: "20", request_id: "candy-41" }],
  };
  render(<PrecheckAccountResult result={fixture} />);
  const summary = screen.getByRole("region", { name: "前置检测结果" });
  expect(within(summary).getAllByText("降智")).toHaveLength(1);
  expect(within(summary).getByText("前置检测").parentElement).toHaveClass("h-8");
  expect(within(summary).queryByRole("list")).not.toBeInTheDocument();
  expect(summary).not.toHaveTextContent("糖果题");
  expect(within(summary).getByRole("button", { name: "查看前置检测详情" })).toBeEnabled();
});

it("两题返回超长回答时卡片仅展示汇总结论，逐题结果和原文在详情中查看", async () => {
  const user = userEvent.setup();
  const fixture = result();
  render(<PrecheckAccountResult result={fixture} />);
  const summary = screen.getByRole("region", { name: "前置检测结果" });
  expect(summary).toHaveClass("h-auto", "shrink-0");
  expect(within(summary).getAllByText("通过")).toHaveLength(1);
  expect(within(summary).queryByRole("list")).not.toBeInTheDocument();
  expect(summary).not.toHaveTextContent("糖果题");
  expect(summary).not.toHaveTextContent(fixture.precheck!.questions[1].answer!);
  expect(summary).not.toHaveTextContent(fixture.model);

  const trigger = within(summary).getByRole("button", { name: "查看前置检测详情" });
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  await user.click(trigger);
  const dialog = await screen.findByRole("dialog", { name: "前置检测详情" });
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  expect(dialog).toHaveTextContent("糖果题");
  expect(dialog).toHaveTextContent(fixture.model);
  expect(dialog).toHaveTextContent("candy-41");
  expect(dialog).toHaveTextContent("cutoff-41");
  expect(within(dialog).getByText(fixture.precheck!.questions[1].answer!)).toHaveClass(
    "wrap-anywhere",
  );
  expect(within(dialog).getByRole("region", { name: "前置检测详细结果" })).toHaveClass(
    "overflow-y-auto",
  );
  await user.click(within(dialog).getByRole("button", { name: "关闭" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(trigger).toHaveFocus();
});

it("单题检测失败时长错误只在详情中展示一次，键盘关闭后返回入口", async () => {
  const user = userEvent.setup();
  const fixture = result();
  const error = `上游请求失败：${"request_payload_too_large_".repeat(80)}`;
  fixture.status = "failed";
  fixture.error = error;
  fixture.precheck = {
    verdict: "error",
    profile_version: "astra-v1",
    questions: [{ id: "candy", verdict: "error", error, request_id: "candy-failed-41" }],
  };
  render(<PrecheckAccountResult result={fixture} />);
  expect(screen.queryByText(error)).not.toBeInTheDocument();
  expect(screen.queryByRole("listitem")).not.toBeInTheDocument();
  expect(screen.getByRole("region", { name: "前置检测结果" })).toHaveClass("h-auto");
  expect(screen.queryByRole("list", { name: "前置检测题目结果" })).not.toBeInTheDocument();
  const trigger = screen.getByRole("button", { name: "查看前置检测详情" });
  trigger.focus();
  await user.keyboard("{Enter}");
  const dialog = await screen.findByRole("dialog", { name: "前置检测详情" });
  expect(within(dialog).getAllByText(error)).toHaveLength(1);
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(trigger).toHaveFocus();
});

it("任务失败且未返回题目结果时，摘要保持紧凑并可查看具体失败原因", async () => {
  const user = userEvent.setup();
  const fixture = {
    ...result(),
    status: "failed" as const,
    precheck: undefined,
    error: "上游返回 503",
  };
  render(<PrecheckAccountResult result={fixture} />);
  const summary = screen.getByRole("region", { name: "前置检测结果" });
  expect(summary).toHaveClass("h-auto");
  expect(summary).toHaveTextContent("检测失败");
  expect(summary).not.toHaveTextContent(fixture.error);
  await user.click(screen.getByRole("button", { name: "查看前置检测详情" }));
  expect(await screen.findByRole("dialog", { name: "前置检测详情" })).toHaveTextContent(
    fixture.error,
  );
});
