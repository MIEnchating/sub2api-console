import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const composeVariables = [
  "COMPOSE_PROJECT_NAME",
  "SUB2API_CONSOLE_API_PORT",
  "SUB2API_CONSOLE_FRONTEND_PORT",
  "SUB2API_CONSOLE_TRUSTED_PROXY_CIDRS",
];

function composeConfig(overrides = {}) {
  const environment = { ...process.env };
  for (const name of composeVariables) {
    delete environment[name];
  }
  Object.assign(environment, overrides);
  const result = spawnSync("docker", ["compose", "config", "--format", "json"], {
    cwd: repositoryRoot,
    encoding: "utf8",
    env: environment,
  });
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout);
}

test("default Compose trust is limited to the project-scoped Unix socket", () => {
  const config = composeConfig();

  assert.equal(config.services.frontend.network_mode, undefined);
  assert.equal(
    config.services.frontend.environment.SUB2API_CONSOLE_API_UPSTREAM,
    "unix:/run/sub2api-console/api.sock",
  );
  assert.equal(
    config.services.api.environment.SUB2API_CONSOLE_TRUSTED_PROXY_SOCKET,
    "/run/sub2api-console/api.sock",
  );
  assert.equal(
    config.services.api.environment.SUB2API_CONSOLE_TRUSTED_PROXY_CIDRS,
    "",
  );
  assert.deepEqual(
    config.services.api.ports.map((port) => [port.host_ip ?? "", port.published, port.target]),
    [["127.0.0.1", "8080", 8080]],
  );
  assert.deepEqual(
    config.services.frontend.ports.map((port) => [port.published, port.target]),
    [["3004", 80]],
  );
  const apiSocket = config.services.api.volumes.find(
    (volume) => volume.target === "/run/sub2api-console",
  );
  const frontendSocket = config.services.frontend.volumes.find(
    (volume) => volume.target === "/run/sub2api-console",
  );
  assert.equal(apiSocket.type, "volume");
  assert.equal(apiSocket.source, "proxy-socket");
  assert.notEqual(apiSocket.read_only, true);
  assert.equal(frontendSocket.source, apiSocket.source);
  assert.equal(frontendSocket.read_only, true);
  assert.equal(config.volumes[apiSocket.source].name, `${config.name}_proxy-socket`);
});

test("Compose keeps explicit host ports and API trust overrides", () => {
  const config = composeConfig({
    COMPOSE_PROJECT_NAME: "parallel-console",
    SUB2API_CONSOLE_API_PORT: "18080",
    SUB2API_CONSOLE_FRONTEND_PORT: "13004",
    SUB2API_CONSOLE_TRUSTED_PROXY_CIDRS: "10.20.30.40/32",
  });

  assert.deepEqual(
    config.services.api.ports.map((port) => [port.published, port.target]),
    [["18080", 8080]],
  );
  assert.deepEqual(
    config.services.frontend.ports.map((port) => [port.published, port.target]),
    [["13004", 80]],
  );
  assert.equal(
    config.services.api.environment.SUB2API_CONSOLE_TRUSTED_PROXY_CIDRS,
    "10.20.30.40/32",
  );
  assert.equal(config.volumes["proxy-socket"].name, "parallel-console_proxy-socket");
});
