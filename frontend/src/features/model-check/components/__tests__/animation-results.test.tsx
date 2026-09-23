import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import type { AnimationResult } from "@/api";
import { AnimationAccountResult } from "../animation-account-result";

const result: AnimationResult = {
  account_id: "41",
  account_name: "测试账号",
  model: "test-model",
  response_model: "returned-model",
  request_id: "request-1",
  status: "succeeded",
  svg: '<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>',
  duration_ms: 1200,
  completed_at: "2026-09-13T00:00:00Z",
};
afterEach(() => vi.useRealTimers());

it("成功结果以图片隔离展示 SVG，并提供模型、耗时和完成时间", () => {
  const view = render(
    <AnimationAccountResult result={result} retryDisabled={false} onRetry={vi.fn()} />,
  );
  const image = screen.getByRole("img", { name: "测试账号生成的鹈鹕骑自行车动画" });
  expect(image).toHaveAttribute(
    "src",
    `data:image/svg+xml;charset=utf-8,${encodeURIComponent(result.svg!)}`,
  );
  expect(image).toHaveClass("w-full", "object-contain");
  expect(view.container.querySelector("circle")).not.toBeInTheDocument();
  expect(screen.getByText(/返回模型 returned-model/)).toBeVisible();
  expect(screen.getByText("模型不一致")).toBeVisible();
  expect(screen.getByText(/耗时 1.2 秒/)).toBeVisible();
});

it("动画持续展示，不提供收起或重新展示按钮", () => {
  vi.useFakeTimers();
  render(<AnimationAccountResult result={result} retryDisabled={false} onRetry={vi.fn()} />);
  act(() => vi.advanceTimersByTime(3_600_000));
  expect(screen.getByRole("img")).toBeVisible();
  expect(screen.queryByRole("button", { name: /收起动画|重新展示动画/ })).not.toBeInTheDocument();
});

it("失败账号展示原因并仅重试对应的账号和模型", () => {
  const retry = vi.fn();
  render(
    <AnimationAccountResult
      result={{ ...result, status: "failed", svg: undefined, error: "上游拒绝请求，请检查余额" }}
      retryDisabled={false}
      onRetry={retry}
    />,
  );
  expect(screen.getByText("上游拒绝请求，请检查余额")).toBeVisible();
  expect(screen.queryByRole("img")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "重试 测试账号" }));
  expect(retry).toHaveBeenCalledWith({ account_id: "41", model: "test-model" });
});

it("自动恢复成功后详情展示重试次数，卡片不增加状态行", async () => {
  const user = userEvent.setup();
  render(
    <AnimationAccountResult
      layout="card"
      result={{ ...result, retry_count: 2 }}
      retryDisabled={false}
      onRetry={vi.fn()}
    />,
  );
  expect(screen.queryByText("自动重试 2 次")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "查看动画检测详情" }));
  expect(screen.getByText("自动重试 2 次")).toBeVisible();
  expect(screen.queryByRole("button", { name: /重试 测试账号/ })).not.toBeInTheDocument();
});

it("检测任务进行中时，失败结果的重试入口禁用", () => {
  render(
    <AnimationAccountResult
      result={{ ...result, status: "failed", svg: undefined, error: "余额不足" }}
      retryDisabled
      onRetry={vi.fn()}
    />,
  );
  expect(screen.getByRole("button", { name: "重试 测试账号" })).toBeDisabled();
});

it("请求与返回模型相同时仅显示一次模型名", () => {
  render(
    <AnimationAccountResult
      result={{ ...result, response_model: result.model }}
      retryDisabled={false}
      onRetry={vi.fn()}
    />,
  );
  expect(screen.getByText(result.model, { exact: true })).toBeVisible();
  expect(screen.queryByText(/返回模型/)).not.toBeInTheDocument();
  expect(screen.queryByText("模型不一致")).not.toBeInTheDocument();
});

it("动画在模型和时间之前展示，紧凑时间保留完整时间提示", () => {
  render(<AnimationAccountResult result={result} retryDisabled={false} onRetry={vi.fn()} />);
  const preview = screen.getByRole("button", { name: /放大查看/ });
  const model = screen.getByText(/返回模型 returned-model/);
  expect(preview.compareDocumentPosition(model) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  const time = screen.getByText("09/13 00:00");
  expect(time).toHaveAttribute("datetime", result.completed_at);
  expect(time).not.toHaveAttribute("title");
});
