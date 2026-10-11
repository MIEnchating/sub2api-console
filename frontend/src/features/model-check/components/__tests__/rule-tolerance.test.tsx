import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { DetectionRule, RuleProbe } from "../../lib/model-check-rules";
import { ModelCheckRuleDetail } from "../model-check-rule-detail";

afterEach(cleanup);

it.each<{ label: string; tolerance: RuleProbe["tolerance"]; expected: string }>([
  {
    label: "相对容差",
    tolerance: { mode: "relative", value: 0.01234 },
    expected: "答案特征值：42；配置容差：1.23%（相对）",
  },
  {
    label: "零绝对容差",
    tolerance: { mode: "absolute", value: 0 },
    expected: "答案特征值：42；配置容差：0（绝对）",
  },
  { label: "未配置容差", tolerance: undefined, expected: "答案特征值：42" },
])("数值题$label 时显示正确单位且不补造容差", async (scenario) => {
  const rule: DetectionRule = {
    id: "numeric-rule",
    label: "测试模型",
    family: "GPT",
    models: ["测试模型"],
    candidates: ["测试模型"],
    identityGroup: [],
    criteria: [{ stage: "首轮", values: [{ label: "覆盖率", value: "80%" }] }],
    questions: [
      {
        stage: "首轮",
        probe: {
          id: "numeric-1",
          kind: "numeric",
          question: "回答数值",
          weights: {},
          clusters: [{ id: "answer", center: 42 }],
          tolerance: scenario.tolerance,
        },
      },
    ],
  };
  const user = userEvent.setup();
  render(<ModelCheckRuleDetail rule={rule} />);
  await user.click(screen.getByRole("tab", { name: "检测题目" }));
  expect(screen.getByText(scenario.expected, { exact: true })).toBeVisible();
});
