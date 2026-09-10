import { requestProfileDetails, type RequestProfile } from "./request-profiles";

type JSONObject = Record<string, unknown>;
function object(value: unknown): value is JSONObject {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
function parseRequestBody(body: string): JSONObject | null {
  try {
    const value: unknown = JSON.parse(body);
    return object(value) ? value : null;
  } catch {
    return null;
  }
}
function userMessage(items: unknown): JSONObject | undefined {
  if (!Array.isArray(items)) return undefined;
  return items.filter((item): item is JSONObject => object(item) && item.role === "user").at(-1);
}
function textBlock(content: unknown): JSONObject | undefined {
  if (!Array.isArray(content)) return undefined;
  return content.find(
    (item): item is JSONObject =>
      object(item) &&
      ["text", "input_text"].includes(String(item.type)) &&
      typeof item.text === "string",
  );
}
export function requestBodyFields(body: string): {
  model: string;
  message: string;
  editable: boolean;
  messageEditable: boolean;
} {
  const value = parseRequestBody(body);
  if (!value) return { model: "", message: "", editable: false, messageEditable: false };
  const content =
    typeof value.input === "string"
      ? value.input
      : userMessage(value.messages ?? value.input)?.content;
  const block = textBlock(content);
  return {
    model: typeof value.model === "string" ? value.model : "",
    message: typeof content === "string" ? content : String(block?.text ?? ""),
    editable: true,
    messageEditable: typeof content === "string" || !!block,
  };
}
export function updateRequestBody(body: string, field: "model" | "message", text: string): string {
  const value = parseRequestBody(body);
  if (!value) return body;
  if (field === "model") value.model = text;
  else if (typeof value.input === "string") value.input = text;
  else {
    const message = userMessage(value.messages ?? value.input);
    if (!message) return body;
    if (typeof message.content === "string") message.content = text;
    else {
      const block = textBlock(message.content);
      if (!block) return body;
      block.text = text;
    }
  }
  return JSON.stringify(value, null, 2);
}
export function requestEndpointURL(value: string, profile: RequestProfile): string {
  if (!value || !profile) return value;
  try {
    const url = new URL(value);
    if (!["http:", "https:"].includes(url.protocol)) return value;
    let path = url.pathname.replace(/\/+$/, "");
    path = path.replace(/\/v1\/(messages?|chat\/completions|responses)$/, "").replace(/\/v1$/, "");
    url.pathname = path + requestProfileDetails[profile].endpoint;
    return url.toString();
  } catch {
    return value;
  }
}
