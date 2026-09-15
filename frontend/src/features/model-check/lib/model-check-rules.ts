import type { ModelCheckAstraProfile, ModelCheckProfilePayload } from "@/api";

export type RuleProbe = Omit<ModelCheckProfilePayload["sol_profile"]["quick"][number], "kind"> & {
  kind: "choice" | "numeric" | "text";
  expected?: string;
};
export type DetectionRule = {
  builtinVersion?: string;
  id: string;
  label: string;
  family: "Claude" | "GPT";
  models: string[];
  candidates: string[];
  identityGroup: string[];
  criteria: Array<{
    stage: string;
    values: Array<{ label: string; value: string; exactValue?: string }>;
  }>;
  questions: Array<{ stage: string; probe: RuleProbe }>;
};

function percent(value: number): string {
  return `${Number((value * 100).toFixed(2))}%`;
}

export function detectionRules(
  payload: ModelCheckProfilePayload,
  astra?: ModelCheckAstraProfile,
): DetectionRule[] {
  const rules: DetectionRule[] = Object.entries(payload.claude_profiles)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([model, profile]) => ({
      id: `claude:${model}`,
      label: model,
      family: "Claude",
      models: [model],
      candidates: profile.candidate_models,
      identityGroup: profile.identity_group,
      criteria: [
        {
          stage: "匹配条件",
          values: [
            {
              label: "回答特征匹配下限",
              value: `≥ ${Number(profile.thresholds[0].toFixed(4))}`,
              exactValue: `≥ ${profile.thresholds[0]}`,
            },
            {
              label: "领先其他候选模型的分差",
              value: `≥ ${Number(profile.thresholds[1].toFixed(4))}`,
              exactValue: `≥ ${profile.thresholds[1]}`,
            },
            { label: "可解析答案占比", value: `≥ ${percent(profile.thresholds[2])}` },
            { label: "有效评分答案占比", value: `≥ ${percent(profile.thresholds[3])}` },
          ],
        },
      ],
      questions: profile.probes.map((probe) => ({ stage: "固定题库", probe })),
    }));
  const sol = payload.sol_profile;
  for (const [index, model] of sol.candidate_models.entries()) {
    rules.push({
      id: `sol:${model}`,
      label: model,
      family: "GPT",
      models: [model],
      candidates: sol.candidate_models,
      identityGroup: [],
      criteria: (["quick", "full"] as const).map((stage) => ({
        stage: stage === "quick" ? "首轮判定" : "补充题后判定",
        values: [
          ...modelCriteria(sol, stage, index),
          { label: "可解析答案占比", value: `≥ ${percent(sol.thresholds[stage].min_coverage)}` },
          {
            label: "有效评分答案占比",
            value: `≥ ${percent(sol.thresholds[stage].min_evidence_coverage)}`,
          },
        ],
      })),
      questions: [
        ...sol.quick.map((probe) => ({ stage: "首轮题目", probe })),
        ...sol.reserve.map((probe) => ({ stage: "补充题目", probe })),
      ],
    });
  }
  if (astra) {
    rules.push({
      id: `astra:${astra.model}`,
      label: astra.model,
      family: "GPT",
      models: [astra.model],
      candidates: [astra.model],
      identityGroup: [],
      builtinVersion: astra.version,
      criteria: [
        {
          stage: "固定规则",
          values: [
            { label: "糖果题", value: "21" },
            { label: "知识截止日期", value: "无法提供日期，且不回答任何日期" },
            { label: "订阅特征", value: "low = 2，mid = 4" },
            { label: "官 Key 特征", value: "low = 4，mid = 10" },
            { label: "来源判定", value: "每轮两档均匹配同一来源，否则无法判定" },
          ],
        },
      ],
      questions: astra.questions.map((question) => ({
        stage: question.effort === "medium" ? "mid（medium）" : "low",
        probe: {
          id: question.id,
          kind: "text",
          question: question.question,
          expected: question.expected,
          weights: {},
        },
      })),
    });
  }
  return rules;
}

function modelCriteria(
  profile: ModelCheckProfilePayload["sol_profile"],
  stage: "quick" | "full",
  index: number,
): Array<{ label: string; value: string }> {
  const model = profile.candidate_models[index];
  const first = profile.candidate_models[0];
  const thresholds = profile.thresholds[stage];
  if (index === 0)
    return [{ label: `${model} 相似度下限`, value: `≥ ${percent(thresholds.sol_accept_min)}` }];
  const other = profile.candidate_models[index === 1 ? 2 : 1];
  return [
    { label: `${first} 尚未达到匹配条件`, value: `< ${percent(thresholds.sol_accept_min)}` },
    { label: `${first} 相似度上限`, value: `≤ ${percent(thresholds.non_sol_accept_max)}` },
    { label: "另外两个候选的相似度之和", value: "> 0" },
    {
      label: `${model} 在另外两个候选中的占比`,
      value: `≥ ${percent(thresholds.subtype_accept_min)}`,
    },
    { label: `与 ${other} 的相似度比较`, value: index === 1 ? "严格大于" : "大于或等于" },
  ];
}
