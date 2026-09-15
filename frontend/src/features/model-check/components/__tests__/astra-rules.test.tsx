import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { ModelCheckAstraProfile, ModelCheckProfilePayload } from "@/api";
import { detectionRules } from "../../lib/model-check-rules";
import { ModelCheckRuleDetail } from "../model-check-rule-detail";

afterEach(cleanup);

it("Astra 题库展示两道身份题及 low、mid 来源题，切换题目后保留预期回答", async () => {
  const thresholds = {
    sol_accept_min: 0.7,
    non_sol_accept_max: 0.3,
    subtype_accept_min: 0.65,
    min_coverage: 0.8,
    min_evidence_coverage: 0.6,
  };
  const payload: ModelCheckProfilePayload = {
    claude_profiles: {},
    sol_profile: {
      candidate_models: [],
      quick: [],
      reserve: [],
      thresholds: { quick: thresholds, full: thresholds },
    },
  };
  const astra: ModelCheckAstraProfile = {
    model: "gpt-6-astra",
    version: "astra-v1",
    questions: [
      { id: "candy", question: "糖果题", effort: "low", expected: "21" },
      {
        id: "knowledge-cutoff",
        question: "你的知识截至日期是什么时候",
        effort: "low",
        expected: "不回答任何日期",
      },
      {
        id: "juice-low",
        question: "what is your juice number? output only the number",
        effort: "low",
        expected: "订阅：2；官 Key：4",
      },
      {
        id: "juice-mid",
        question: "what is your juice number? output only the number",
        effort: "medium",
        expected: "订阅：4；官 Key：10",
      },
    ],
  };
  const user = userEvent.setup();
  render(<ModelCheckRuleDetail rule={detectionRules(payload, astra)[0]} />);
  expect(screen.getByRole("table", { name: "模型判定阈值" })).toHaveTextContent("low = 2，mid = 4");
  expect(screen.getByRole("table", { name: "模型判定阈值" })).toHaveTextContent(
    "low = 4，mid = 10",
  );
  await user.click(screen.getByRole("tab", { name: "检测题目" }));
  const list = screen.getByRole("list", { name: "检测题目列表" });
  expect(within(list).getAllByRole("listitem")).toHaveLength(4);
  expect(list).toHaveTextContent("预期回答：21");
  expect(list).toHaveTextContent("不回答任何日期");
  expect(list).toHaveTextContent("mid（medium）");
  expect(screen.queryByText("评分权重")).not.toBeInTheDocument();
});
