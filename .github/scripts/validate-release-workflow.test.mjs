import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../..", import.meta.url));
const workflow = readFileSync(`${root}/.github/workflows/release.yml`, "utf8");

test("release workflow builds one multi-architecture Docker Hub image", () => {
  assert.match(workflow, /IMAGE: mienvirtuoso\/sub2api-console/);
  assert.match(workflow, /context: \./);
  assert.match(workflow, /file: backend\/Dockerfile/);
  assert.match(workflow, /platforms: linux\/amd64,linux\/arm64/);
  assert.match(workflow, /DOCKERHUB_TOKEN/);
  assert.doesNotMatch(workflow, /ghcr\.io|packages:/);
  assert.equal((workflow.match(/docker\/build-push-action@/g) ?? []).length, 1);
  assert.match(workflow, /create-release:/);
  assert.match(workflow, /needs: build-and-push/);
  assert.match(workflow, /contents: write/);
  assert.match(workflow, /gh release create/);
  assert.match(workflow, /\.github\/release-notes\/\$GITHUB_REF_NAME\.md/);
});
