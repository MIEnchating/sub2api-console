import type { Sub2APIModelPrice } from "@/api";

export type NewAPIManagementView = "groups" | "channels" | "prices" | "differences";

export const modelPriceSourceLabels = {
  official: "官方价格",
  remote: "远程价卡",
  sub2api: "Sub2API 默认",
} as const satisfies Record<NonNullable<Sub2APIModelPrice["source"]>, string>;

export const rawPricingModeLabels: Record<string, string> = {
  chat: "对话",
  completion: "文本补全",
  embedding: "向量嵌入",
  image_generation: "图像生成",
  audio_transcription: "语音识别",
  audio_speech: "语音合成",
  video_generation: "视频生成",
  rerank: "重排序",
  moderation: "内容审核",
};

export const rawPricingFieldLabels: Record<string, string> = {
  litellm_provider: "厂商",
  mode: "模型类型",
  max_input_tokens: "最大输入 Token",
  max_output_tokens: "最大输出 Token",
  max_tokens: "Token 上限",
  prompt_cache_min_tokens: "缓存最小 Token 数",
  deprecation_date: "停用日期",
  source: "来源",
  supported_endpoints: "支持的接口",
  supported_modalities: "输入类型",
  supported_output_modalities: "输出类型",
  supports_function_calling: "工具调用",
  supports_parallel_function_calling: "并行工具调用",
  supports_tool_choice: "工具选择",
  supports_prompt_caching: "提示词缓存",
  supports_vision: "视觉输入",
  supports_response_schema: "结构化响应",
  supports_native_structured_output: "原生结构化输出",
  supports_system_messages: "系统消息",
  supports_pdf_input: "PDF 输入",
  supports_reasoning: "推理",
  supports_web_search: "联网搜索",
  supports_native_streaming: "流式输出",
  supports_audio_input: "音频输入",
  supports_audio_output: "音频输出",
  supports_assistant_prefill: "回复预填",
  supports_computer_use: "计算机操作",
  supports_url_context: "网页上下文",
  supports_xhigh_reasoning_effort: "超高推理强度",
  supports_minimal_reasoning_effort: "最低推理强度",
  supports_none_reasoning_effort: "关闭推理",
};

export const rawPricingTokenLabels: Record<string, string> = {
  input_cost_per_token: "输入",
  output_cost_per_token: "输出",
  cache_read_input_token_cost: "缓存读取",
  cache_creation_input_token_cost: "缓存写入",
  cache_creation_input_token_cost_above_1hr: "缓存写入（1 小时以上）",
  input_cost_per_image_token: "图像输入",
  output_cost_per_image_token: "图像输出",
  input_cost_per_audio_token: "音频输入",
  output_cost_per_audio_token: "音频输出",
};

export const rawPricingOtherPriceLabels: Record<string, string> = {
  input_cost_per_image: "图像输入（每张）",
  output_cost_per_image: "图像输出（每张）",
  input_cost_per_second: "输入（每秒）",
  output_cost_per_second: "输出（每秒）",
  input_cost_per_audio_per_second: "音频输入（每秒）",
};
