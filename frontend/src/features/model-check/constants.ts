import type { PrecheckQuestionID } from "@/api";

export const allPrecheckQuestions: PrecheckQuestionID[] = ["candy", "knowledge-cutoff"];

export function precheckQuestionSummary(questions = allPrecheckQuestions): string {
  return questions.map((id) => (id === "candy" ? "糖果题" : "知识截止日期题")).join("和");
}

export const astraSourceLabels: Record<string, string> = {
  subscription: "订阅特征",
  official_key: "官 Key 特征",
  inconclusive: "来源无法判定",
};

export const precheckVerdictLabels = {
  passed: "通过",
  not_passed: "不通过",
  inconclusive: "无法判定",
  error: "请求失败",
};

export const precheckQuestionLabels: Record<string, string> = {
  candy: "糖果题",
  "knowledge-cutoff": "知识截止日期",
};
