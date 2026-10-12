import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath, URL } from "node:url";
import process from "node:process";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));

function composeConfig(external = false, driver = "postgres", dsn = "") {
  const files = external ? ["-f", "docker-compose.yml", "-f", "docker-compose.external.yml"] : [];
  const result = spawnSync("docker", ["compose", ...files, "config", "--format", "json"], {
    cwd: repositoryRoot,
    encoding: "utf8",
    env: {
      ...process.env,
      SUB2API_CONSOLE_FRONTEND_PORT: "3004",
      SUB2API_CONSOLE_DB_DRIVER: driver,
      SUB2API_CONSOLE_DB_DSN: dsn,
      SUB2API_CONSOLE_POSTGRES_PASSWORD: "contract-only-password",
    },
  });
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout);
}

test("Compose runs protocol authorization without a browser service or socket dependency", () => {
  const config = composeConfig();
  assert.deepEqual(Object.keys(config.services).sort(), ["api", "postgres"]);
  assert.equal(config.services.api.image, "mienvirtuoso/sub2api-console:latest");
  assert.deepEqual(
    config.services.api.ports.map((port) => [port.published, port.target]),
    [["3004", 8080]],
  );
  assert.equal(config.services.api.depends_on.postgres.condition, "service_healthy");
  assert.equal(
    config.services.api.volumes.some((volume) => volume.target === "/run/browser"),
    false,
  );
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

test("the env template exposes database selection without shipping secrets", () => {
  const template = readFileSync(`${repositoryRoot}/.env.example`, "utf8");
  const variables = [...template.matchAll(/^(SUB2API_CONSOLE_[A-Z_]+)=/gm)].map(
    (match) => match[1],
  );
  assert.deepEqual(variables.sort(), [
    "SUB2API_CONSOLE_DB_DRIVER",
    "SUB2API_CONSOLE_DB_DSN",
    "SUB2API_CONSOLE_DB_MAX_IDLE_CONNS",
    "SUB2API_CONSOLE_DB_MAX_OPEN_CONNS",
    "SUB2API_CONSOLE_FRONTEND_PORT",
    "SUB2API_CONSOLE_POSTGRES_PASSWORD",
    "SUB2API_CONSOLE_SETUP_TOKEN",
  ]);
  assert.match(template, /^SUB2API_CONSOLE_POSTGRES_PASSWORD=$/m);
  assert.match(template, /^SUB2API_CONSOLE_DB_DSN=$/m);
  assert.match(template, /^SUB2API_CONSOLE_SETUP_TOKEN=$/m);
});

test("default PostgreSQL has private networking, persistent storage and matching credentials", () => {
  const config = composeConfig();
  assert.equal(config.services.api.environment.SUB2API_CONSOLE_DB_DRIVER, "postgres");
  const connection = new URL(config.services.api.environment.SUB2API_CONSOLE_DB_DSN);
  assert.equal(connection.hostname, "postgres");
  assert.equal(connection.password, config.services.postgres.environment.POSTGRES_PASSWORD);
  assert.equal(config.services.postgres.ports, undefined);
  assert.equal(config.services.postgres.volumes[0].type, "volume");
  assert.equal(config.services.postgres.volumes[0].target, "/var/lib/postgresql");
});

test("external database and SQLite overrides remove the bundled PostgreSQL dependency", () => {
  for (const [driver, dsn] of [
    ["sqlite", ""],
    ["mysql", "user:fixture@tcp(mysql.test:3306)/console"],
  ]) {
    const config = composeConfig(true, driver, dsn);
    assert.deepEqual(Object.keys(config.services), ["api"]);
    assert.deepEqual(config.services.api.depends_on ?? {}, {});
    assert.equal(config.services.api.environment.SUB2API_CONSOLE_DB_DRIVER, driver);
    assert.equal(config.services.api.environment.SUB2API_CONSOLE_DB_DSN, dsn);
  }
});
