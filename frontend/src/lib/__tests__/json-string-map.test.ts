import { describe, expect, it } from "vitest";

import { parseJsonStringMap } from "../json-string-map";

describe("parseJsonStringMap", () => {
  it("returns a string-only JSON object", () => {
    expect(parseJsonStringMap('{"Authorization":"Bearer secret"}', "Headers")).toEqual({
      Authorization: "Bearer secret",
    });
  });

  it.each(['{"X-User":9}', '{"X-Nested":{"enabled":true}}', '{"":"value"}'])(
    "rejects a map that cannot be sent as string headers: %s",
    (value) => {
      expect(() => parseJsonStringMap(value, "Headers")).toThrow(
        "Headers 的名称不能为空，值必须是字符串",
      );
    },
  );
});
