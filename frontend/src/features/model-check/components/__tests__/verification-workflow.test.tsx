import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { VerificationResult } from "../verification-result";

it("历史未进行对照提示隐藏但实际指纹采样仍可查看", async () => {
  render(
    <VerificationResult
      result={{
        version: "verification-v3",
        source: "third_party",
        capability: { status: "passed" },
        authenticity: { status: "insufficient_evidence", summary: "旧泛化提示" },
        reference: { status: "not_run", summary: "未选择官方 API 对照账号，本次未进行对照。" },
        fingerprint: {
          status: "stable",
          summary: "重复采样稳定",
          samples: [
            {
              id: "chinese",
              label: "中文文本",
              status: "stable",
              first: 130,
              repeat: 130,
              delta: 30,
            },
          ],
        },
        total: 17,
        capability_total: 7,
        passed: 7,
      }}
    />,
  );
  expect(screen.queryByText("官方对照未进行")).not.toBeInTheDocument();
  expect(screen.getByText("指纹采集稳定")).toBeVisible();
  expect(screen.queryByText("身份依据不足")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "查看检测证据" }));
  const dialog = screen.getByRole("dialog", { name: "检测证据" });
  await userEvent.click(within(dialog).getByRole("button", { name: "来源与指纹" }));
  expect(within(dialog).getByRole("region", { name: "分词指纹采样" })).toHaveTextContent(
    "中文文本",
  );
  expect(within(dialog).getByRole("region", { name: "分词指纹采样" })).toHaveTextContent("130");
  expect(within(dialog).getByRole("region", { name: "分词指纹采样" })).toHaveTextContent("30");
  const sampling = within(dialog).getByRole("region", { name: "分词指纹采样" });
  expect(within(sampling).getByRole("cell", { name: "稳定" })).toBeVisible();
  expect(within(sampling).getByRole("table")).toHaveClass("whitespace-nowrap", "min-w-72");
});

it("官方指纹对照出现差异时展示两侧数值与差值而不是笼统身份告警", async () => {
  render(
    <VerificationResult
      result={{
        reference: { status: "mismatch", summary: "中文差分不同" },
        fingerprint: { status: "stable", samples: [] },
        reference_comparisons: [
          {
            account_id: "2",
            status: "mismatch",
            summary: "不同",
            samples: [{ label: "中文文本", observed: 38, reference: 30, difference: 8 }],
          },
        ],
      }}
    />,
  );
  expect(screen.getByText("官方对照存在差异")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "查看检测证据" }));
  const dialog = screen.getByRole("dialog", { name: "检测证据" });
  await userEvent.click(within(dialog).getByRole("button", { name: "来源与指纹" }));
  const comparison = within(dialog).getByRole("region", { name: "官方指纹对照明细" });
  expect(comparison).toHaveTextContent("参考账号 2");
  expect(comparison).toHaveTextContent("38");
  expect(comparison).toHaveTextContent("30");
  expect(comparison).toHaveTextContent("8");
});

it("历史报告只有泛化身份不足结论时说明未记录对照状态且不继续显示身份问题", () => {
  render(
    <VerificationResult
      result={{
        version: "verification-v2",
        capability: { status: "passed" },
        authenticity: { status: "insufficient_evidence", summary: "身份依据不足，请重新检测。" },
      }}
    />,
  );
  expect(screen.queryByText(/身份依据不足/)).not.toBeInTheDocument();
  expect(screen.queryByText(/官方对照/)).not.toBeInTheDocument();
});

it("关闭指纹的同题对照相符时不冒充指纹认证", () => {
  render(
    <VerificationResult
      result={{
        reference: {
          status: "matched",
          summary: "同题正确性与原始用量对照完成；本次未采集分词差分。",
        },
        fingerprint: { status: "not_run" },
      }}
    />,
  );
  expect(screen.getByText("官方对照相符")).toBeVisible();
  expect(screen.getByText("指纹未采集")).toBeVisible();
});

it("独立检测不再展示未进行官方对照或引导添加参考账号", () => {
  render(
    <VerificationResult
      result={{
        version: "verification-v4",
        authenticity: { status: "observed", summary: "接口检查已完成。" },
        capability: { status: "passed" },
        fingerprint: { status: "stable" },
      }}
    />,
  );
  expect(screen.queryByText(/官方对照|对照证据/)).not.toBeInTheDocument();
  expect(screen.getByText("指纹采集稳定")).toBeVisible();
});
