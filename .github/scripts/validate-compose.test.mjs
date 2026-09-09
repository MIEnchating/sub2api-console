import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

function composeConfig() {
  const result = spawnSync("docker", ["compose", "config", "--format", "json"], { cwd: repositoryRoot, encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout);
}

test("Compose defines one Docker Hub application image", () => {
  const config = composeConfig();
  assert.deepEqual(Object.keys(config.services), ["api"]);
  assert.equal(config.services.api.image, "mienvirtuoso/sub2api-console:latest");
  assert.deepEqual(config.services.api.ports.map((port) => [port.published, port.target]), [["3004", 8080]]);
});

test("Compose keeps application data persistent and internal paths fixed", () => {
  const config = composeConfig();
  assert.equal(config.services.api.volumes[0].type, "bind");
  assert.equal(config.services.api.volumes[0].source, join(repositoryRoot, "data"));
  assert.equal(config.services.api.volumes[0].target, "/app/data");
  assert.equal(config.services.api.environment.SUB2API_CONSOLE_DATA_DIR, undefined);
  assert.equal(config.services.api.environment.SUB2API_CONSOLE_DATA_DB, undefined);
  assert.equal(config.services.api.environment.SUB2API_CONSOLE_TASK_DB, undefined);
  assert.equal(config.services.api.environment.SUB2API_CONSOLE_CONFIG_DB, undefined);
});

test("the env template exposes only the public port and setup token", () => {
  const template = readFileSync(`${repositoryRoot}/.env.example`, "utf8");
  const variables = [...template.matchAll(/^(SUB2API_CONSOLE_[A-Z_]+)=/gm)].map((match) => match[1]);
  assert.deepEqual(variables.sort(), ["SUB2API_CONSOLE_FRONTEND_PORT", "SUB2API_CONSOLE_SETUP_TOKEN"]);
});
