import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { OnboardingProbeModelField } from "../onboarding-probe-model-field";

afterEach(cleanup);
it("模型列表尚未就绪时不崩溃，列表到达后恢复探活选择", () => {
  const props = {
    fieldId: "probe-models",
    identity: "测试账号",
    value: "",
    disabled: false,
    onChange: () => {},
    onBlur: () => {},
  };
  const view = render(<OnboardingProbeModelField {...props} models={undefined} />);
  expect(screen.getByRole("combobox", { name: "测试账号 探活模型" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  view.rerender(<OnboardingProbeModelField {...props} models={["gpt-5"]} />);
  expect(screen.getByRole("combobox", { name: "测试账号 探活模型" })).not.toHaveAttribute(
    "aria-disabled",
    "true",
  );
});
