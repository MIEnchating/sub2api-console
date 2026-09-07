import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

for (const [filename, job] of [["ci.yml", "frontend"], ["release.yml", "preflight"]]) {
  test(`${filename} requires browser regressions and dead code checks before publishing`, () => {
    const workflow = readFileSync(new URL(`../workflows/${filename}`, import.meta.url), "utf8");
    const body = `${workflow}\n  __end__:\n`.match(new RegExp(`^  ${job}:\\n([\\s\\S]*?)(?=^  [a-zA-Z0-9_]+:\\n)`, "m"))?.[1];
    assert.ok(body, `missing mandatory ${job} job`);
    assert.match(body, /bun x playwright install --with-deps chromium/);
    assert.match(body, /bun run test:e2e/);
    assert.match(body, /bun run deadcode/);
    assert.doesNotMatch(body, /continue-on-error:\s*true/);
    assert.ok(body.indexOf("playwright install") < body.indexOf("bun run test:e2e"));
  });
}
