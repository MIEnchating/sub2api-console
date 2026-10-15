import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { VerificationResult } from "../verification-result";

it("能力通过但接口信息未记录时分别展示结论且可打开原始证据", async () => {
  render(
    <VerificationResult
      result={{
        authenticity: { status: "unverified", summary: "第三方身份尚未证实" },
        capability: { status: "passed", summary: "本次全部通过" },
        passed: 7,
        total: 7,
        evidence: [
          { id: "model", label: "返回型号", status: "consistent", detail: "型号字段不是身份证明" },
        ],
        probes: [
          {
            id: "arithmetic",
            label: "多步计算",
            round: 1,
            request_id: "request-1",
            status: "passed",
            prompt: "计算 7 × 3",
            expected: "21",
            answer: "**21**",
          },
        ],
      }}
    />,
  );
  expect(screen.getByText("接口信息未记录")).toBeVisible();
  expect(screen.getByText("接口信息未记录").closest('[data-slot="badge"]')).toHaveAttribute(
    "data-variant",
    "neutral",
  );
  expect(screen.getByText("能力通过")).toBeVisible();
  expect(screen.queryByText(/100%/)).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "查看检测证据" }));
  const dialog = screen.getByRole("dialog", { name: "检测证据" });
  expect(within(dialog).getByText("**21**")).toBeVisible();
  expect(within(dialog).getByText(/输入 — \/ 输出 — Token/)).toBeVisible();
  await userEvent.click(within(dialog).getByRole("button", { name: "来源与指纹" }));
  expect(within(dialog).getByText("型号字段不是身份证明")).toBeVisible();
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: "检测证据" })).not.toBeInTheDocument();
});

it("请求限流时展示能力待判定而不是能力失败", () => {
  render(
    <VerificationResult
      result={{
        authenticity: { status: "unverified", summary: "身份尚未证实" },
        capability: { status: "inconclusive", summary: "请求限流，请稍后重试" },
        passed: 0,
        total: 7,
        evidence: [],
        probes: [],
      }}
    />,
  );
  expect(screen.getByText("能力待判定")).toBeVisible();
  expect(screen.queryByText("能力未通过")).not.toBeInTheDocument();
});

it("没有逐项记录时保留证据切换和关闭入口", async () => {
  render(<VerificationResult result={{ probes: [], evidence: [] }} />);
  await userEvent.click(screen.getByRole("button", { name: "查看检测证据" }));
  const dialog = screen.getByRole("dialog", { name: "检测证据" });
  expect(within(dialog).getByText("没有逐项记录")).toBeVisible();
  await userEvent.click(within(dialog).getByRole("button", { name: "来源与指纹" }));
  expect(within(dialog).getByText(/能力、请求来源与指纹稳定性分别展示/)).toBeVisible();
  await userEvent.click(within(dialog).getByRole("button", { name: "关闭" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
