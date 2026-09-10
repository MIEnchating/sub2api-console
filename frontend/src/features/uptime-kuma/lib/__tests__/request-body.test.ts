import { expect, it } from "vitest";
import { requestBodyFields, updateRequestBody, requestEndpointURL } from "../request-body";

it("修改 Claude 消息时保留系统提示、图片和其他消息", () => {
  const body = JSON.stringify({
    model: "old",
    system: "keep",
    max_tokens: 16,
    messages: [
      { role: "user", content: "prior" },
      { role: "assistant", content: "reply" },
      {
        role: "user",
        content: [
          { type: "image", source: { type: "url", url: "https://example.test/image" } },
          { type: "text", text: "old" },
        ],
      },
    ],
  });
  expect(requestBodyFields(body)).toMatchObject({
    model: "old",
    message: "old",
    messageEditable: true,
  });
  const result = JSON.parse(updateRequestBody(body, "message", "new"));
  expect(result).toMatchObject({
    system: "keep",
    max_tokens: 16,
    messages: [
      { role: "user", content: "prior" },
      { role: "assistant", content: "reply" },
      {
        role: "user",
        content: [
          { type: "image", source: { type: "url", url: "https://example.test/image" } },
          { type: "text", text: "new" },
        ],
      },
    ],
  });
});
it.each([
  '{"model":"old","input":"ping","store":false}',
  '{"model":"old","input":[{"role":"user","content":[{"type":"input_text","text":"ping"}]}],"store":false}',
])("Responses 文本输入和数组输入都可以修改且保留参数 %s", (body) => {
  const result = updateRequestBody(
    updateRequestBody(body, "model", "new-model"),
    "message",
    "new-message",
  );
  expect(requestBodyFields(result)).toMatchObject({ model: "new-model", message: "new-message" });
  expect(JSON.parse(result).store).toBe(false);
});
it("无效 JSON 或不支持的消息结构不被快捷编辑覆盖", () => {
  expect(requestBodyFields("broken").editable).toBe(false);
  expect(updateRequestBody("broken", "model", "new")).toBe("broken");
  const body = '{"input":[{"type":"item_reference","id":"keep"}]}';
  expect(requestBodyFields(body).messageEditable).toBe(false);
  expect(updateRequestBody(body, "message", "new")).toBe(body);
});
it.each([
  "https://example.test",
  "https://example.test/v1",
  "https://example.test/v1/message",
  "https://example.test/v1/chat/completions/",
])("地址 %s 按接口模式补全且不重复追加", (url) => {
  const result = requestEndpointURL(url, "claude-messages");
  expect(result).toBe("https://example.test/v1/messages");
  expect(requestEndpointURL(result, "claude-messages")).toBe(result);
});
it("切换接口保留网关前缀和查询参数，自定义模式不改写地址", () => {
  expect(
    requestEndpointURL("https://example.test/proxy/v1/messages?key=keep", "openai-responses"),
  ).toBe("https://example.test/proxy/v1/responses?key=keep");
  expect(requestEndpointURL("https://example.test/custom", "")).toBe("https://example.test/custom");
  expect(requestEndpointURL("", "openai-chat")).toBe("");
});
