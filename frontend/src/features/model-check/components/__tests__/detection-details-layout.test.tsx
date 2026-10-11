import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import type { AnimationResult } from "@/api";
import { AnimationResultDetails } from "../animation-result-details";
import { PrecheckResultDetails } from "../precheck-result-details";

const result: AnimationResult = {
  account_id: "41",
  account_name: "检测账号",
  model: "gpt-6-astra",
  response_model: "gpt-6-astra",
  request_id: "details-41",
  status: "succeeded",
  duration_ms: 1500,
  generation_duration_ms: 1000,
  completed_at: "2026-10-10T00:31:00Z",
  reasoning_effort: "medium",
  usage: { input_tokens: 100, output_tokens: 40, total_tokens: 140, reasoning_tokens: 20 },
  precheck: {
    verdict: "passed",
    profile_version: "astra-v1",
    questions: [{ id: "candy", verdict: "passed", answer: "21", request_id: "candy-41" }],
  },
};

it.each(["前置检测", "动画检测"])(
  "%s 详情按两列及桌面三列展示六项统计，模型耗时只出现一次且没有费用",
  async (stage) => {
    const user = userEvent.setup();
    render(
      stage === "前置检测" ? (
        <PrecheckResultDetails result={result} />
      ) : (
        <AnimationResultDetails result={result} />
      ),
    );
    await user.click(screen.getByRole("button", { name: `查看${stage}详情` }));
    const dialog = screen.getByRole("dialog", { name: `${stage}详情` });
    expect(dialog).toHaveClass("sm:max-w-3xl");
    const detail = within(dialog);
    expect(detail.getAllByText(result.model, { exact: true })).toHaveLength(1);
    expect(detail.getAllByText("耗时", { exact: true })).toHaveLength(1);
    expect(detail.getAllByText(/^请求 ID[：:]?$/)).toHaveLength(1);
    expect(detail.queryByText("candy-41")).not.toBeInTheDocument();
    expect(detail.getAllByText("1.5 秒", { exact: true })).toHaveLength(1);
    const metrics = detail.getByRole("group", { name: "用量与性能" });
    expect(metrics).toHaveClass("grid-cols-2", "sm:grid-cols-3");
    expect(within(metrics).getAllByRole("term")).toHaveLength(6);
    expect(within(metrics).getByText("推理 Token")).toBeVisible();
    expect(within(metrics).getByText("20", { exact: true })).toBeVisible();
    expect(detail.queryByText(/预估费用|无法估算/)).not.toBeInTheDocument();
    const body = detail.getByRole("region", { name: `${stage}详细结果` });
    expect(body).toContainElement(metrics);
    expect(body).toHaveClass("overflow-y-auto");
    expect(detail.getByRole("region", { name: "请求信息" })).toHaveTextContent(result.request_id);
    expect(detail.getByLabelText("完成时间")).toHaveAttribute("datetime", result.completed_at);
    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: `查看${stage}详情` })).toHaveFocus();
  },
);

it.each(["前置检测", "动画检测"])(
  "%s 请求失败且无用量时显示真实失败原因，保留缺失值提示",
  async (stage) => {
    const failed: AnimationResult = {
      ...result,
      status: "failed",
      error: "上游连接返回 HTTP 400：请求被拒绝",
      usage: undefined,
      precheck: undefined,
      response_model: "returned-model",
    };
    const user = userEvent.setup();
    render(
      stage === "前置检测" ? (
        <PrecheckResultDetails result={failed} />
      ) : (
        <AnimationResultDetails result={failed} />
      ),
    );
    await user.click(screen.getByRole("button", { name: `查看${stage}详情` }));
    const dialog = within(screen.getByRole("dialog", { name: `${stage}详情` }));
    expect(dialog.getByText(failed.error!)).toBeVisible();
    expect(dialog.getAllByText("未返回")).toHaveLength(4);
    expect(dialog.getByText("无法计算")).toBeVisible();
    expect(dialog.getByText("重点：模型不一致")).toBeVisible();
    expect(dialog.getByText("returned-model")).toHaveClass("wrap-anywhere");
    expect(dialog.queryByText(/预估费用|无法估算/)).not.toBeInTheDocument();
  },
);
