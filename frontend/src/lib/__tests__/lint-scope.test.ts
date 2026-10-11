import { ESLint } from "eslint";
import { expect, it } from "vitest";

it("旧部署构建备份不进入 lint，但正式源码与浏览器回归仍受检查", async () => {
  const eslint = new ESLint();
  expect(await eslint.isPathIgnored("dist.previous-deploy-20261008T0420Z/static/js/index.js")).toBe(
    true,
  );
  expect(await eslint.isPathIgnored("src/features/logs/components/logs-center-page.tsx")).toBe(
    false,
  );
  expect(await eslint.isPathIgnored("e2e/__tests__/logs-center-layout.e2e.ts")).toBe(false);
  const results = await eslint.lintText("const unused: number = 1;", {
    filePath: "src/features/logs/lint-fixture.ts",
  });
  expect(
    results[0].messages.some((message) => message.ruleId === "@typescript-eslint/no-unused-vars"),
  ).toBe(true);
});
