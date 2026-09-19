import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const quality = readFileSync(new URL("../workflows/quality.yml", import.meta.url), "utf8");
function job(workflow, name) {
  const body = `${workflow}\n  __end__:\n`.match(
    new RegExp(`^  ${name}:\\n([\\s\\S]*?)(?=^  [a-zA-Z0-9_-]+:\\n)`, "m"),
  )?.[1];
  assert.ok(body, `missing ${name} job`);
  return body;
}
for (const filename of ["ci.yml", "release.yml"]) {
  test(`${filename} uses the complete shared quality workflow`, () => {
    const workflow = readFileSync(new URL(`../workflows/${filename}`, import.meta.url), "utf8");
    assert.match(job(workflow, "checks"), /uses: \.\/\.github\/workflows\/quality.yml/);
    assert.doesNotMatch(quality, /continue-on-error:\s*true/);
  });
}
test("all four browser shards run independently and preserve separate failure diagnostics", () => {
  const browser = job(quality, "browser");
  assert.match(browser, /shard: \[1, 2, 3, 4\]/);
  assert.match(browser, /fail-fast: false/);
  assert.match(browser, /bun run test:e2e --shard=\$\{\{ matrix.shard \}\}\/4/);
  assert.ok(browser.indexOf("playwright install") < browser.indexOf("bun run test:e2e"));
  assert.match(browser, /failure\(\) && steps\.browser\.outcome == 'failure'/);
  assert.match(browser, /name: browser-failure-diagnostics-\$\{\{ matrix.shard \}\}/);
  assert.match(browser, /path: frontend\/test-results\//);
  for (const name of ["browser", "frontend", "backend"])
    assert.doesNotMatch(job(quality, name), /needs:/);
});
test("shared quality checks retain all mandatory commands and discover new script tests", () => {
  for (const command of ["format:check", "typecheck", "lint", "deadcode", "test", "build"]) {
    assert.ok(job(quality, "frontend").includes(`bun run ${command}`));
  }
  assert.match(job(quality, "backend"), /bash scripts\/check-go\.sh vet/);
  assert.match(job(quality, "backend"), /bash scripts\/check-go\.sh test/);
  assert.match(job(quality, "backend"), /go build /);
  assert.match(
    job(quality, "configuration"),
    /node --test \.github\/scripts\/\*\.test\.mjs \.github\/scripts\/__tests__\/\*\.test\.mjs/,
  );
});
test("publication and release creation explicitly require successful upstream gates", () => {
  const workflow = readFileSync(new URL("../workflows/release.yml", import.meta.url), "utf8");
  assert.match(job(workflow, "checks"), /if: needs.source.outputs.reuse != 'true'/);
  assert.match(job(workflow, "preflight"), /needs: \[source, checks\]/);
  assert.match(
    job(workflow, "build-and-push"),
    /!cancelled\(\) && needs.preflight.result == 'success'/,
  );
  assert.match(
    job(workflow, "create-release"),
    /!cancelled\(\) && needs.build-and-push.result == 'success'/,
  );
});
