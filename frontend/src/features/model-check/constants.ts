import { statusToneClasses } from "@/lib/status-styles";
import type { PrecheckQuestionID } from "@/api";
import type { AccountSort } from "@/features/accounts/lib/account-sort";

export const detectionStageOptions = [
  { value: "animation", label: "动画检测", description: "生成并预览动画结果" },
  { value: "precheck", label: "前置检测", description: "使用糖果题检查回答" },
] as const;

export const animationPlatforms = ["openai", "anthropic", "gemini"] as const;
export const animationPlatformLabels = {
  openai: "OpenAI",
  anthropic: "Anthropic",
  gemini: "Gemini",
} as const;
export function supportsAnimationPlatform(platform: string): boolean {
  return animationPlatforms.some((supported) => supported === platform);
}
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

export const animationManualVerdictLabels = {
  passed: "不降智",
  not_passed: "降智",
  unmarked: "未标记",
} as const;

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
  success: statusToneClasses.success,
  danger: statusToneClasses.danger,
  warning: statusToneClasses.warning,
  muted: statusToneClasses.neutral,
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

export const animationAccountSortOptions = [
  { value: "default", label: "默认排序" },
  { value: "priority_asc", label: "优先级：从小到大" },
  { value: "priority_desc", label: "优先级：从大到小" },
  { value: "weight_desc", label: "权重：从大到小" },
  { value: "weight_asc", label: "权重：从小到大" },
] as const satisfies ReadonlyArray<{ value: AccountSort; label: string }>;
export type AnimationAccountSort = (typeof animationAccountSortOptions)[number]["value"];

export const animationQualityLabels = {
  normal: "正常特征",
  suspicious: "疑似低质量",
  unknown: "无法判定",
} as const;

export const animationQualityReasons: Record<string, string> = {
  review_failed: "评审请求失败，请检查评审账号和模型设置后重新检测。",
  review_timeout: "评审等待或请求超时，请检查评审服务或调整超时后重新检测。",
  review_cancelled: "评审已取消，动画生成结果已保留。",
  review_invalid_response: "评审模型未返回有效标签和理由，请更换评审模型后重新检测。",
  review_account_unavailable: "评审账号不可用或已被排除，请重新配置评审账号。",
  review_settings_changed: "评审期间统一设置已变化，请重新检测以使用新设置。",
  model_unavailable: "模型未配置，先积累人工标注样本",
  source_unavailable: "缺少完整有效的生成原文",
  prompt_unavailable: "该历史记录未保存提示词",
  unsupported_benchmark: "该提示词尚无适用的已验证模型",
  unsupported_format: "当前分类器仅支持完整 HTML",
  no_known_features: "源码未命中模型特征",
  ambiguous_score: "模型得分位于拒判区间",
};

export const verificationCapabilityLabels: Record<string, string> = {
  passed: "能力通过",
  failed: "能力未通过",
  inconclusive: "能力待判定",
};
export const verificationProbeLabels: Record<string, string> = {
  passed: "通过",
  failed: "未通过",
  error: "请求失败",
};
export const verificationProtocolLabels: Record<string, string> = {
  anthropic_messages: "Anthropic Messages",
  openai_responses: "OpenAI Responses",
  openai_chat: "OpenAI Chat",
  gemini_native: "Gemini 原生",
  other: "其他协议",
};
