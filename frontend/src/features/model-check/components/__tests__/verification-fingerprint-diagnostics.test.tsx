import { render, screen, within } from "@testing-library/react";
import { expect, it } from "vitest";
import { VerificationFingerprintDetails } from "../verification-fingerprint-details";

it("指纹不可用时显示每项具体缺失字段与首次复测型号和协议", () => {
  render(
    <VerificationFingerprintDetails
      result={{
        fingerprint: {
          status: "unavailable",
          samples: [
            {
              id: "baseline",
              label: "基准文本",
              status: "unavailable",
              first: 59,
              repeat: 59,
              detail: "首次未返回型号；复测未返回型号",
              protocol: "openai_responses",
              repeat_protocol: "openai_responses",
            },
          ],
        },
      }}
    />,
  );
  const region = screen.getByRole("region", { name: "分词指纹采样" });
  expect(within(region).getByText("首次未返回型号；复测未返回型号")).toBeVisible();
  expect(within(region).getByText(/首次型号：未返回/)).toBeVisible();
  expect(within(region).getByText(/复测型号：未返回/)).toBeVisible();
  expect(within(region).getByText(/OpenAI Responses/)).toBeVisible();
});
it("旧报告缺少诊断字段时从保存的逐项记录显示缺失型号和用量变化", () => {
  render(
    <VerificationFingerprintDetails
      result={{
        fingerprint: {
          status: "unavailable",
          samples: [
            { id: "english", label: "英文文本", status: "unavailable", first: 436, repeat: 79 },
          ],
        },
        probes: [
          { id: "fingerprint_english", status: "observed", protocol: "openai_responses" },
          { id: "fingerprint_english_repeat", status: "observed", protocol: "openai_responses" },
        ],
      }}
    />,
  );
  expect(screen.getByText(/436 → 79/)).toBeVisible();
  expect(screen.getByText(/首次未返回型号/)).toBeVisible();
  expect(screen.getByText(/复测未返回型号/)).toBeVisible();
});
it("旧报告样本稳定但没有差分时说明基准条件未满足", () => {
  render(
    <VerificationFingerprintDetails
      result={{
        fingerprint: {
          status: "unavailable",
          samples: [{ id: "code", label: "代码文本", status: "stable", first: 4290, repeat: 4290 }],
        },
      }}
    />,
  );
  expect(screen.getByText(/未形成基准差分/)).toBeVisible();
});
