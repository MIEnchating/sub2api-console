export const requestProfiles = [
  "",
  "claude-cli",
  "claude-messages",
  "openai-chat",
  "openai-responses",
] as const;
export type RequestProfile = (typeof requestProfiles)[number];
export const requestProfileLabels: Record<RequestProfile, string> = {
  "": "自定义请求",
  "claude-cli": "Claude CLI 请求",
  "claude-messages": "Claude Messages",
  "openai-chat": "OpenAI Chat Completions",
  "openai-responses": "OpenAI Responses",
};
export const requestProfileDetails: Record<
  Exclude<RequestProfile, "">,
  { endpoint: string; model: string }
> = {
  "claude-cli": {
    endpoint: "/v1/messages",
    model: "claude-sonnet-4-6",
  },
  "claude-messages": {
    endpoint: "/v1/messages",
    model: "claude-sonnet-4-6",
  },
  "openai-chat": {
    endpoint: "/v1/chat/completions",
    model: "gpt-4.1-mini",
  },
  "openai-responses": {
    endpoint: "/v1/responses",
    model: "gpt-4.1-mini",
  },
};
export function parseRequestProfile(value?: string): RequestProfile {
  return requestProfiles.find((profile) => profile === value) ?? "";
}
