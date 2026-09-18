import { expect, it } from "vitest";
import { probeFallbackLabel } from "../display";

it("启用目录更新时展示启用目录的首个模型", () => {
  expect(
    probeFallbackLabel({
      known_models: ["known"],
      enabled_models: ["enabled"],
      known_models_synced_at: "2026-01-01T00:00:00Z",
      enabled_models_synced_at: "2026-01-02T00:00:00Z",
    }),
  ).toContain("enabled");
});
it("已知目录较新时展示已知目录的首个模型", () => {
  expect(
    probeFallbackLabel({
      known_models: ["known"],
      enabled_models: ["enabled"],
      known_models_synced_at: "2026-01-03T00:00:00Z",
      enabled_models_synced_at: "2026-01-02T00:00:00Z",
    }),
  ).toContain("known");
});
it("同步时间缺失时保留后端优先启用目录的回退", () => {
  expect(probeFallbackLabel({ known_models: ["known"], enabled_models: ["enabled"] })).toContain(
    "enabled",
  );
});
it("账号无可用目录时要求同步模型，不伪造默认模型", () => {
  expect(probeFallbackLabel({ known_models: [null, " "], enabled_models: [] })).toBe(
    "暂无已同步的可用模型，请先同步账号模型",
  );
});
