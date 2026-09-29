import type { PrecheckQuestionID } from "@/api";

export const detectionStageOptions = [
  { value: "animation", label: "动画检测", description: "生成并预览动画结果" },
  { value: "precheck", label: "前置检测", description: "使用糖果题检查回答" },
  { value: "terminal", label: "终端检测", description: "多轮检测终端续接能力" },
] as const;

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

export type DetectionResultTone = "success" | "danger" | "warning" | "muted";

export const detectionMissingResultStates = {
  waiting: { label: "等待检测结果", tone: "muted" },
  timeout: {
    label: "任务超时，未取得结果",
    tone: "warning",
    reason: "任务已超时结束；没有结果的阶段可能尚未执行或执行中断，请重新检测。",
  },
  cancelled: {
    label: "任务已取消，未取得结果",
    tone: "muted",
    reason: "任务已取消；没有结果的阶段可能尚未执行或执行中断，需要时可重新检测。",
  },
  failed: {
    label: "任务失败，未取得结果",
    tone: "warning",
    reason: "任务失败且未记录具体原因；没有结果的阶段无法确认是否执行，请重新检测。",
  },
  missing: {
    label: "结果未记录",
    tone: "warning",
    reason: "任务已结束，但未保存该阶段结果，无法确认是否执行；请重新检测。",
  },
} as const;

export const detectionResultToneClasses: Record<DetectionResultTone, string> = {
  success: "border-success/30 bg-success/10 text-success",
  danger: "border-destructive/30 bg-destructive/10 text-destructive",
  warning: "border-warning/30 bg-warning/10 text-warning",
  muted: "border-border bg-muted/30 text-muted-foreground",
};

export const precheckVerdictTones = {
  passed: "success",
  not_passed: "danger",
  inconclusive: "warning",
  error: "danger",
} as const satisfies Record<keyof typeof precheckVerdictLabels, DetectionResultTone>;

export const terminalVerdictTones = {
  normal: "success",
  suspected: "danger",
  inconclusive: "warning",
  error: "danger",
} as const satisfies Record<keyof typeof terminalVerdicts, DetectionResultTone>;

export const animationResultStatuses = {
  succeeded: { label: "成功", tone: "success" },
  failed: { label: "失败", tone: "danger" },
} as const;

export const precheckQuestionLabels: Record<string, string> = {
  candy: "糖果题",
};

export const animationTestModel = "gpt-6-astra";
