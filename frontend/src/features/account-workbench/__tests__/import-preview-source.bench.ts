import { bench } from "vitest";

import { matchesImportPreviewSource } from "../lib/import-preview-source";
import { importDefaults } from "../lib/import-schema";

for (const size of [1024, 4 * 1024 * 1024]) {
  const source = { ...importDefaults, content: "x".repeat(size) };
  const current = { ...source };
  const serialized = JSON.stringify(source);
  bench(
    `序列化比较-${size}`,
    () => {
      if (JSON.stringify(current) !== serialized) throw new Error("资料比较失败");
    },
    { time: 1000, warmupTime: 300 },
  );
  bench(
    `字段比较-${size}`,
    () => {
      if (!matchesImportPreviewSource(current, source)) throw new Error("资料比较失败");
    },
    { time: 1000, warmupTime: 300 },
  );
}
