import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { AnimationQualityBadge } from "../animation-quality-badge";

afterEach(cleanup);

it.each([
  "review_failed",
  "review_timeout",
  "review_request_timeout",
  "review_queue_timeout",
  "review_task_timeout",
  "review_invalid_response",
  "review_account_unavailable",
  "review_settings_changed",
])("评审执行失败 %s 时显示失败而非无法判定", (reason) => {
  render(
    <AnimationQualityBadge
      prediction={{ status: "unknown", score: null, benchmark: "test", reason }}
    />,
  );
  expect(screen.getByLabelText("自动质量：评审失败")).toHaveTextContent("评审失败");
  expect(screen.queryByText("无法判定")).not.toBeInTheDocument();
});
it("评审证据不足时保留无法判定，不误报请求失败", () => {
  render(
    <AnimationQualityBadge
      prediction={{
        status: "unknown",
        score: null,
        benchmark: "test",
        reason: "review_insufficient_evidence",
      }}
    />,
  );
  expect(screen.getByLabelText("自动质量：无法判定")).toBeVisible();
});
it("人工已标注正常时不继承自动评审失败状态", () => {
  render(
    <AnimationQualityBadge
      manual
      prediction={{ status: "normal", score: null, benchmark: "test", reason: "review_failed" }}
    />,
  );
  expect(screen.getByLabelText("质量标注：正常")).toBeVisible();
});
it("取消评审时展示已取消，不作为质量结论", () => {
  render(
    <AnimationQualityBadge
      prediction={{ status: "unknown", score: null, benchmark: "test", reason: "review_cancelled" }}
    />,
  );
  expect(screen.getByLabelText("自动质量：评审已取消")).toBeVisible();
});
