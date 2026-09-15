import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { ProbeProgressSummary } from "../probe-task-timeline";
import { ProbeResultSlot } from "../account-probe-dialog";

afterEach(cleanup);

it("进度默认仅展示当前阶段，键盘展开后可查看全部真实步骤", async () => {
  const user = userEvent.setup();
  render(
    <ProbeProgressSummary
      steps={[
        { stage: "create_key", status: "succeeded", started_at: "2026-09-11T00:00:00Z" },
        { stage: "models", status: "running", started_at: "2026-09-11T00:00:01Z" },
      ]}
    />,
  );
  const toggle = screen.getByRole("button", { name: /获取上游模型列表/ });
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByRole("list", { name: "探活过程" })).not.toBeInTheDocument();
  toggle.focus();
  await user.keyboard("{Enter}");
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByRole("list", { name: "探活过程" })).toBeInTheDocument();
  expect(screen.getByText("创建临时上游 Key")).toBeVisible();
});

it("测试尚未开始时结果区域默认展示当前模型和等待提示", () => {
  render(<ProbeResultSlot pending={false} error={null} result={null} requestModel="kimi-k2" />);
  expect(screen.getByText(/kimi-k2/)).toBeVisible();
  expect(screen.getByText("点击“开始测试”查看响应")).toBeVisible();
});

it("清理失败时折叠摘要直接显示失败阶段", () => {
  render(
    <ProbeProgressSummary
      steps={[{ stage: "cleanup_key", status: "failed", started_at: "2026-09-11T00:00:00Z" }]}
    />,
  );
  expect(screen.getByRole("button", { name: /清理临时上游 Key.*失败/ })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
});

it("请求失败但清理成功时摘要仍显示请求失败", () => {
  render(
    <ProbeProgressSummary
      steps={[
        { stage: "request", status: "failed", started_at: "2026-09-15T00:00:00Z" },
        { stage: "cleanup_key", status: "succeeded", started_at: "2026-09-15T00:00:01Z" },
      ]}
    />,
  );
  expect(screen.getByRole("button", { name: /发送探活请求并等待响应.*失败/ })).toBeVisible();
});

it("响应与过程切换时保持紧凑高度，收起后恢复已有结果", async () => {
  const user = userEvent.setup();
  render(
    <ProbeProgressSummary
      steps={[{ stage: "cleanup_key", status: "succeeded", started_at: "2026-09-15T00:00:00Z" }]}
    >
      <ProbeResultSlot
        pending={false}
        error={null}
        result={{
          status: "passed",
          message: "完成",
          request_model: "model",
          actual_model: "model",
          response_text: "本次探活响应",
          http_status: 200,
          latency_ms: 100,
        }}
      />
    </ProbeProgressSummary>,
  );
  expect(screen.getByText("本次探活响应")).toBeVisible();
  const toggle = screen.getByRole("button", { name: /清理临时上游 Key/ });
  const detailPanel = document.getElementById(toggle.getAttribute("aria-controls")!);
  expect(detailPanel).toHaveClass("h-48", "overflow-hidden");
  await user.click(toggle);
  expect(screen.getByRole("list", { name: "探活过程" })).toBeVisible();
  expect(detailPanel).toHaveClass("h-48", "overflow-hidden");
  expect(screen.queryByText("本次探活响应")).not.toBeInTheDocument();
  await user.click(toggle);
  expect(screen.getByText("本次探活响应")).toBeVisible();
});

it("长探活响应使用可键盘访问的独立滚动区并保留真实模型信息", () => {
  render(
    <ProbeResultSlot
      pending={false}
      error={null}
      result={{
        status: "passed",
        message: "完成",
        request_model: "model-".repeat(80),
        actual_model: "actual-model",
        response_text: "response ".repeat(500),
        http_status: 200,
        latency_ms: 250,
      }}
    />,
  );
  const output = screen.getByRole("region", { name: "探活响应内容" });
  expect(output).toHaveAttribute("tabindex", "0");
  expect(output).toHaveClass("overflow-y-auto", "overscroll-contain");
  expect(output).toHaveClass("min-h-0", "flex-1");
  expect(screen.getByText(/actual-model/)).toBeVisible();
  expect(screen.queryByText(/发送测试消息："hi"/)).not.toBeInTheDocument();
});
