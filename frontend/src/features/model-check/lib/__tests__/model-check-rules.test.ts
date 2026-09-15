import { expect, it } from "vitest";
import type { ModelCheckProfilePayload } from "@/api";
import { detectionRules } from "../model-check-rules";

it("长小数阈值展示四位小数并保留完整原值，不改变题库配置", () => {
  const source = payload();
  source.claude_profiles["claude-test"] = {
    identity_group: ["claude-test"],
    candidate_models: ["claude-test", "claude-other"],
    thresholds: [-1.331790694578457, -0.5465200463006162, 0.6, 0.5],
    score_bands: [],
    probes: [],
  };
  const rule = detectionRules(source)[0];
  expect(rule.criteria[0].values[0]).toEqual({
    label: "回答特征匹配下限",
    value: "≥ -1.3318",
    exactValue: "≥ -1.331790694578457",
  });
  expect(rule.criteria[0].values[1]).toEqual({
    label: "领先其他候选模型的分差",
    value: "≥ -0.5465",
    exactValue: "≥ -0.5465200463006162",
  });
  expect(source.claude_profiles["claude-test"].thresholds[0]).toBe(-1.331790694578457);
});

function payload(): ModelCheckProfilePayload {
  const thresholds = {
    sol_accept_min: 0.7,
    non_sol_accept_max: 0.3,
    subtype_accept_min: 0.65,
    min_coverage: 0.8,
    min_evidence_coverage: 0.6,
  };
  return {
    claude_profiles: {},
    sol_profile: {
      candidate_models: ["gpt-5.6-sol", "gpt-5.6-luna", "gpt-5.6-terra"],
      quick: [{ id: "sample", kind: "numeric", question: "数值", weights: { c0: [1, 2, 3] } }],
      reserve: [],
      thresholds: { quick: thresholds, full: { ...thresholds, sol_accept_min: 0.75 } },
    },
  };
}

it("三个 GPT 模型各有独立规则入口，题目保留原始候选权重对应关系", () => {
  const rules = detectionRules(payload());
  expect(rules.map((rule) => rule.label)).toEqual(["gpt-5.6-sol", "gpt-5.6-luna", "gpt-5.6-terra"]);
  expect(new Set(rules.map((rule) => rule.id)).size).toBe(3);
  for (const rule of rules) {
    expect(rule.models).toEqual([rule.label]);
    expect(rule.questions[0].probe.weights.c0[rule.candidates.indexOf(rule.label)]).toBe(
      { "gpt-5.6-sol": 1, "gpt-5.6-luna": 2, "gpt-5.6-terra": 3 }[rule.label],
    );
  }
});

it("Sol 规则使用各阶段自己的相似度下限，不混入其他模型的接受条件", () => {
  const rule = detectionRules(payload())[0];
  expect(rule.criteria[0].values).toContainEqual({
    label: "gpt-5.6-sol 相似度下限",
    value: "≥ 70%",
  });
  expect(rule.criteria[1].values).toContainEqual({
    label: "gpt-5.6-sol 相似度下限",
    value: "≥ 75%",
  });
  expect(rule.criteria[0].values.some((entry) => entry.label.includes("luna"))).toBe(false);
});

it("Luna 和 Terra 规则展示各自占比及领先条件，保留后端对平分的不同处理", () => {
  const rules = detectionRules(payload());
  expect(rules[1].criteria[0].values).toContainEqual({
    label: "gpt-5.6-luna 在另外两个候选中的占比",
    value: "≥ 65%",
  });
  expect(rules[1].criteria[0].values).toContainEqual({
    label: "与 gpt-5.6-terra 的相似度比较",
    value: "严格大于",
  });
  expect(rules[2].criteria[0].values).toContainEqual({
    label: "gpt-5.6-terra 在另外两个候选中的占比",
    value: "≥ 65%",
  });
  expect(rules[2].criteria[0].values).toContainEqual({
    label: "与 gpt-5.6-luna 的相似度比较",
    value: "大于或等于",
  });
});
