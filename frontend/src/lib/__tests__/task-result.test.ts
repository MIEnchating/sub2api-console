import { describe, expect, it } from "vitest";

import { flattenTaskResult } from "../task-result";

describe("task result presentation", () => {
  it("preserves explicit false, zero, empty text, null, list, and object values", () => {
    expect(
      flattenTaskResult({ enabled: false, count: 0, detail: "", data: null, items: [], meta: {} }),
    ).toEqual([
      { path: ["enabled"], value: "关闭" },
      { path: ["count"], value: "0" },
      { path: ["detail"], value: "空值" },
      { path: ["data"], value: "空值" },
      { path: ["items"], value: "空列表" },
      { path: ["meta"], value: "空对象" },
    ]);
  });

  it("flattens nested results into labeled leaves instead of JSON text", () => {
    expect(flattenTaskResult({ pending: { upstream_key_id: "99" } })).toEqual([
      { path: ["pending", "upstream_key_id"], value: "99" },
    ]);
  });
});
