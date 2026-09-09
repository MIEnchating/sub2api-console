import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

for (const [filename, job] of [["ci.yml", "frontend"], ["release.yml", "preflight"]]) {
  test(`${filename} requires browser regressions and dead code checks before publishing`, () => {
    const workflow = readFileSync(new URL(`../workflows/${filename}`, import.meta.url), "utf8");
    const body = `${workflow}\n  __end__:\n`.match(new RegExp(`^  ${job}:\\n([\\s\\S]*?)(?=^  [a-zA-Z0-9_-]+:\\n)`, "m"))?.[1];
    assert.ok(body, `missing mandatory ${job} job`);
    assert.match(body, /bun x playwright install --with-deps chromium/);
    assert.match(body, /bun run test:e2e/);
    assert.match(body, /bun run deadcode/);
    assert.doesNotMatch(body, /continue-on-error:\s*true/);
    assert.ok(body.indexOf("playwright install") < body.indexOf("bun run test:e2e"));
    assert.match(body, /failure\(\) && steps\.browser\.outcome == 'failure'/);
    assert.match(body, /uses: actions\/upload-artifact@/);
    assert.match(body, /path: frontend\/test-results\//);
  });
}

test("release publishing waits for successful frontend and backend preflight checks", () => {
  const workflow = readFileSync(new URL("../workflows/release.yml", import.meta.url), "utf8");
  const preflight = workflow.match(/^  preflight:\n([\s\S]*?)(?=^  [a-zA-Z0-9_-]+:\n)/m)?.[1];
  const publish = workflow.match(/^  build-and-push:\n([\s\S]*?)(?=^  [a-zA-Z0-9_-]+:\n)/m)?.[1];
  assert.ok(preflight, "missing release preflight checks");
  assert.ok(publish, "missing image publication job");
  assert.match(publish, /needs: preflight/);
  assert.doesNotMatch(publish, /if:.*always\(\)/);
  for (const command of ["format:check", "typecheck", "lint", "test", "build"]) {
    assert.ok(preflight.includes(`bun run ${command}`), `missing frontend ${command}`);
  }
  assert.match(preflight, /go vet /);
  assert.match(preflight, /go test -race /);
  assert.match(preflight, /go build /);
  assert.match(preflight, /node --test \.github\/scripts\/\*\.test\.mjs/);
});
