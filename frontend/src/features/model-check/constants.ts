import type { PrecheckQuestionID } from "@/api";

export const animationPlatformLabels = { openai: "OpenAI", anthropic: "Anthropic" } as const;
export const terminalVerdicts = {
  normal: { label: "正常", variant: "secondary" },
  suspected: { label: "疑似无终端权限", variant: "destructive" },
  inconclusive: { label: "证据不足", variant: "warning" },
  error: { label: "请求失败", variant: "destructive" },
} as const;

export const allPrecheckQuestions: PrecheckQuestionID[] = ["candy"];

export function precheckQuestionSummary(questions = allPrecheckQuestions): string {
  return questions.map(() => "糖果题").join("和");
}

export const astraSourceLabels: Record<string, string> = {
  subscription: "订阅特征",
  official_key: "官 Key 特征",
  inconclusive: "来源无法判定",
};

export const precheckVerdictLabels = {
  passed: "通过",
  not_passed: "降智",
  inconclusive: "无法判定",
  error: "请求失败",
};

export const precheckQuestionLabels: Record<string, string> = {
  candy: "糖果题",
};
