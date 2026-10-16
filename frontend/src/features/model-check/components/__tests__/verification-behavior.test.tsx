import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { VerificationBehaviorDetails } from "../verification-behavior-details";
import { VerificationCombinationProgress } from "../verification-combination-progress";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("采样完整时展示分布及零差异并保持身份无法判定", () => {
  render(
    <VerificationBehaviorDetails
      result={{
        behavior_fingerprint: {
          version: "short-answer-distribution-v1",
          status: "collected",
          valid: 200,
          planned: 200,
          summary: "无法判定模型身份",
          cells: [
            {
              id: "coin",
              label: "硬币",
              status: "collected",
              valid: 25,
              invalid: 0,
              errors: 0,
              missing: 0,
              self_jsd: 0,
              counts: { heads: 25 },
            },
          ],
        },
      }}
    />,
  );
  const section = screen.getByRole("region", { name: "行为指纹采样" });
  expect(within(section).getByText("heads: 25")).toBeVisible();
  expect(within(section).getByText("0.000")).toBeVisible();
  expect(within(section).getByText("无法判定模型身份")).toBeVisible();
});

it("采样不足时分别展示无效、失败和未执行且不把缺失差异补零", () => {
  render(
    <VerificationBehaviorDetails
      result={{
        behavior_fingerprint: {
          version: "v1",
          status: "incomplete",
          valid: 3,
          planned: 200,
          cells: [
            {
              id: "coin",
              label: "硬币",
              status: "incomplete",
              valid: 3,
              invalid: 2,
              errors: 1,
              missing: 19,
              counts: { heads: 3 },
            },
          ],
        },
      }}
    />,
  );
  const row = screen.getByRole("row", { name: /硬币/ });
  expect(
    within(row)
      .getAllByRole("cell")
      .map((cell) => cell.textContent),
  ).toEqual(["3", "2", "1", "19", "—", "heads: 3"]);
});

it("旧报告没有行为指纹时不显示采样结论", () => {
  render(<VerificationBehaviorDetails result={{}} />);
  expect(screen.queryByRole("region", { name: "行为指纹采样" })).not.toBeInTheDocument();
});

it("行为采样在实时过程独立分组且不归入能力题", () => {
  render(
    <VerificationCombinationProgress
      active
      result={{
        total: 1,
        probes: [
          {
            id: "behavior",
            label: "硬币采样",
            round: 1,
            category: "behavior_fingerprint",
            status: "observed",
          },
        ],
      }}
    />,
  );
  expect(screen.getByRole("region", { name: "行为指纹" })).toBeVisible();
  expect(screen.queryByRole("region", { name: "能力检测" })).not.toBeInTheDocument();
});
