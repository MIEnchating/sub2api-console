import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { VerificationResult } from "../verification-result";

it.each([
  ["official_source", "官方来源已确认", "success"],
  ["reference_consistent", "对照未见差异", "success"],
  ["evidence_mismatch", "存在待核对差异", "warning"],
  ["observed", "接口检查完成", "success"],
  ["inconclusive", "接口检测未完成", "neutral"],
])("后端返回 %s 时结果与证据展示对应状态和原因", async (status, label, variant) => {
  render(
    <VerificationResult
      result={{
        authenticity: { status, summary: "本次来源与对照的具体依据" },
        capability: { status: "passed" },
      }}
    />,
  );
  expect(screen.getByText(label).closest('[data-slot="badge"]')).toHaveAttribute(
    "data-variant",
    variant,
  );
  await userEvent.click(screen.getByRole("button", { name: "查看检测证据" }));
  const dialog = screen.getByRole("dialog", { name: "检测证据" });
  expect(within(dialog).getByText(label)).toBeVisible();
  expect(within(dialog).getByText("本次来源与对照的具体依据")).toBeVisible();
});

it.each([
  {},
  { authenticity: { status: "unverified" } },
  { authenticity: { status: "future-status" } },
])("历史或未知状态不推测身份认证并提示重新检测", (result) => {
  render(<VerificationResult result={result} />);
  expect(screen.getByText("接口信息未记录")).toBeVisible();
  expect(screen.getByText(/历史报告未记录完整接口结论.*重新检测/)).toBeVisible();
});
