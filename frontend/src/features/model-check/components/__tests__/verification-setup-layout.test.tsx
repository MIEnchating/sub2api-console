import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { VerificationSetup } from "../verification-setup";

it("模型检测统一说明能力指纹和真伪分析，不再要求单独选择检测手段", () => {
  render(<VerificationSetup />);
  const steps = screen.getByRole("region", { name: "检测步骤" });
  expect(steps).toHaveTextContent("能力检测、指纹采样与真伪分析一次完成");
  expect(steps).toHaveTextContent("根据所选模型自动匹配公开参考");
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
});
