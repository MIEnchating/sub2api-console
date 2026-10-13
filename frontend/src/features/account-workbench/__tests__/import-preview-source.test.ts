import { expect, it } from "vitest";

import { matchesImportPreviewSource } from "../lib/import-preview-source";
import { importDefaults, importSchema, type ImportValues } from "../lib/import-schema";

const source: ImportValues = { ...importDefaults, content: "rt_fixture" };

it("资料和模型的首尾空白规范化后仍匹配同一预览", () => {
  const current = importSchema.parse({
    ...source,
    content: " rt_fixture\n",
    model: ` ${source.model} `,
  });
  expect(matchesImportPreviewSource(current, source)).toBe(true);
});

const changes: { [Key in keyof ImportValues]: ImportValues[Key] } = {
  content: "rt_changed",
  action: "export",
  template_id: "another-template",
  check: false,
  promote: false,
  model: "another-model",
  proxy_enabled: true,
  platform_proxy_id: "3",
  proxy_url: "https://proxy.example.test",
};

it.each(Object.keys(changes) as (keyof ImportValues)[])("修改 %s 后原预览不再匹配", (key) => {
  expect(matchesImportPreviewSource({ ...source, [key]: changes[key] }, source)).toBe(false);
});

it("字段插入顺序不同的相同资料仍匹配预览", () => {
  const current = Object.fromEntries(Object.entries(source).reverse()) as ImportValues;
  expect(matchesImportPreviewSource(current, source)).toBe(true);
});

it("4 MB 资料末尾内容变更使旧预览失效", () => {
  const content = "a".repeat(4 * 1024 * 1024);
  const large = { ...source, content };
  expect(matchesImportPreviewSource({ ...large }, large)).toBe(true);
  expect(matchesImportPreviewSource({ ...large, content: content.slice(0, -1) + "b" }, large)).toBe(
    false,
  );
});
