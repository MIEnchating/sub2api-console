import { expect, it } from "vitest";
import { modelMatchesBlockPatterns } from "../model-block-patterns";

it.each([
  { model: "GPT-5", patterns: [" gpt-5 "], blocked: true },
  { model: "gpt-5-mini", patterns: ["gpt-5"], blocked: false },
  { model: "gpt-5-mini", patterns: ["gpt-*-mini"], blocked: true },
  { model: "gpt-5", patterns: ["gpt-5*"], blocked: true },
  { model: "model-😀", patterns: ["model-?"], blocked: true },
  { model: "model-ab", patterns: ["model-?"], blocked: false },
  { model: "model-5x1", patterns: ["model-5.1"], blocked: false },
  { model: "model-[5.1]", patterns: ["model-[5.1]"], blocked: true },
  { model: "gpt-5", patterns: [], blocked: false },
])("模型 $model 按规则 $patterns 匹配时屏蔽结果为 $blocked", (fixture) => {
  expect(modelMatchesBlockPatterns(fixture.model, fixture.patterns)).toBe(fixture.blocked);
});
