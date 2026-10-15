import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { VerificationResult } from "../verification-result";
import { VerificationFingerprintDetails } from "../verification-fingerprint-details";
import { VerificationStateBadge } from "../verification-state";

it("状态文案较长时可通过键盘聚焦读取完整提示", async () => {
  const label = "接口检查完成，已采集当前接口的完整响应证据";
  render(<VerificationStateBadge status="observed" label={label} />);
  await userEvent.setup().tab();
  expect(await screen.findByRole("tooltip")).toHaveTextContent(label);
});

it("能力通过、接口完成和指纹稳定在结果与证据中使用一致的成功图标和对齐布局", async () => {
  render(
    <VerificationResult
      result={{
        capability: { status: "passed" },
        authenticity: { status: "observed" },
        fingerprint: { status: "stable" },
      }}
    />,
  );
  for (const label of ["能力通过", "接口检查完成", "指纹采集稳定"]) {
    const text = screen.getByText(label);
    const badge = text.closest('[data-slot="badge"]');
    expect(badge).toHaveAttribute("data-variant", "success");
    expect(text.parentElement).toHaveClass("inline-flex", "items-center", "gap-1");
    expect(badge?.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
    expect(badge?.querySelector("svg")).toHaveClass("shrink-0");
  }
  await userEvent.setup().click(screen.getByRole("button", { name: "查看检测证据" }));
  const dialog = screen.getByRole("dialog", { name: "检测证据" });
  for (const label of ["能力通过", "接口检查完成", "指纹采集稳定"]) {
    expect(within(dialog).getByText(label).closest('[data-slot="badge"]')).toHaveAttribute(
      "data-variant",
      "success",
    );
  }
});

it("指纹明细中稳定、不稳定和不可用状态均有图标并按真实状态着色", () => {
  render(
    <VerificationFingerprintDetails
      result={{
        fingerprint: {
          status: "unstable",
          samples: [
            { id: "base", label: "基准文本", status: "stable" },
            { id: "chinese", label: "中文文本", status: "unstable" },
            { id: "english", label: "英文文本", status: "unavailable" },
          ],
        },
      }}
    />,
  );
  for (const [label, variant] of [
    ["稳定", "success"],
    ["不稳定", "warning"],
    ["不可用", "neutral"],
  ]) {
    const badge = screen.getByRole("cell", { name: label }).querySelector('[data-slot="badge"]');
    expect(badge).toHaveAttribute("data-variant", variant);
    expect(badge?.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  }
});

it("指纹单项采集完成在列表与详情使用成功状态且保留已采集含义", async () => {
  render(
    <VerificationResult
      result={{
        probes: [
          {
            id: "sample",
            category: "fingerprint",
            label: "基准采样",
            status: "observed",
            answer: "OK",
          },
        ],
      }}
    />,
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "查看检测证据" }));
  const detail = screen.getByRole("article", { name: "检测项详情" });
  const badge = within(detail).getByText("已采集").closest('[data-slot="badge"]');
  expect(badge).toHaveAttribute("data-variant", "success");
  const item = screen.getByRole("button", { name: /第.*轮.*已采集.*基准采样/ });
  expect(item.querySelector("svg")).toHaveClass("text-success");
});
