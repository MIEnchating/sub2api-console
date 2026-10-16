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

export const animationQualityVerdicts = {
  normal: "不降智",
  suspicious: "降智",
  unknown: "未知",
} as const;

export const animationQualityLabels = {
  normal: "正常",
  suspicious: "降智",
  unknown: "无法判定",
} as const;

export const animationQualityReasons: Record<string, string> = {
  review_failed: "评审请求失败，请检查评审账号和模型设置后重新评审，无需重新生成动画。",
  review_timeout: "本次评审已超时；该历史记录未区分排队与请求。可重新评审，无需重新生成动画。",
  review_request_timeout: "评审请求超时，请检查评审服务或调整评审超时后重新评审。",
  review_queue_timeout: "排队期间所属任务已超时，尚未发送评审请求。可重新评审。",
  review_task_timeout: "所属任务已超时，评审已中断。可重新评审。",
  review_cancelled: "评审已取消，动画生成结果已保留。",
  review_invalid_response: "评审模型返回格式无效，未获得有效结论。请重新评审或人工标注。",
  review_insufficient_evidence:
    "评审结论缺少可核对的题目或原文依据，或与证据矛盾，请重新评审或人工复核。",
  review_account_unavailable: "评审账号不可用或已被排除，请重新配置评审账号。",
  review_settings_changed: "评审期间统一设置已变化，请重新评审以使用新设置。",
  model_unavailable: "模型未配置，先积累人工标注样本",
  source_unavailable: "缺少完整有效的生成原文",
  prompt_unavailable: "该历史记录未保存提示词",
  unsupported_benchmark: "该提示词尚无适用的已验证模型",
  unsupported_format: "当前分类器仅支持完整 HTML",
  no_known_features: "源码未命中模型特征",
  ambiguous_score: "模型得分位于拒判区间",
};

export const animationQualityFailureLabels: Record<string, string> = {
  review_failed: "评审失败",
  review_timeout: "评审失败",
  review_request_timeout: "评审失败",
  review_queue_timeout: "评审失败",
  review_task_timeout: "评审失败",
  review_invalid_response: "评审失败",
  review_account_unavailable: "评审失败",
  review_settings_changed: "评审失败",
  review_cancelled: "评审已取消",
};

export const verificationCapabilityLabels: Record<string, string> = {
  passed: "能力通过",
  failed: "能力未通过",
  inconclusive: "能力待判定",
};
export const verificationIdentityLabels: Record<string, string> = {
  observed: "接口检查完成",
  official_source: "官方来源已确认",
  reference_consistent: "对照未见差异",
  evidence_mismatch: "存在待核对差异",
  insufficient_evidence: "接口信息未记录",
  inconclusive: "接口检测未完成",
};
export const verificationIdentityLegacyExplanation =
  "历史报告未记录完整接口结论，可重新检测获取；现有答题结果继续保留，不据此判断模型真假。";
export const verificationIdentityExplanation =
  "能力、请求来源与指纹稳定性分别展示；答题通过和采样稳定不等于具体模型权重认证。";
export const verificationReferenceLabels: Record<string, string> = {
  not_run: "官方对照未进行",
  matched: "官方对照相符",
  mismatch: "官方对照存在差异",
  incomplete: "官方对照未完成",
  not_applicable: "官方来源已确认",
};
export const verificationFingerprintSampleLabels: Record<string, string> = {
  stable: "稳定",
  unstable: "不稳定",
  incomplete: "未完成",
  unavailable: "不可用",
};

export const verificationFingerprintLabels: Record<string, string> = {
  stable: "指纹采集稳定",
  unstable: "指纹采样不稳定",
  incomplete: "指纹采集未完成",
  unavailable: "指纹数据不可用",
  not_run: "指纹未采集",
};
export const verificationProbeLabels: Record<string, string> = {
  observed: "已采集",
  running: "等待回复",
  pending: "待执行",
  skipped: "未执行",
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

export const detectionTimeoutLabels = {
  animation: "动画超时",
  precheck: "前置检测超时",
  review: "评审超时",
} as const;

export const animationQualityPendingLabel = "质量标注中";

export const verificationBehaviorLabels: Record<string, string> = {
  collected: "采样完整",
  incomplete: "样本不完整",
  incomparable: "采样不可比",
};

export const publicReferenceLabels: Record<string, string> = {
  matched: "与公开参考一致",
  mismatch: "与公开参考显著不符（疑似冒充）",
  inconclusive: "公开参考：无法判定",
};
export const publicReferenceMethodLabels: Record<string, string> = {
  knowledge: "知识边界比较",
  tokenizer: "公开分词对照",
  distribution: "回答分布比较",
};
export const publicReferenceEvidenceLabels: Record<string, string> = {
  consistent: "一致",
  different: "有差异",
  unstable: "不稳定",
  evidence_only: "仅提供差异证据",
  inconclusive: "无法判定",
};
