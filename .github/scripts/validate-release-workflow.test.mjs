import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { chmodSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const workflow = readFileSync(new URL("../workflows/release.yml", import.meta.url), "utf8");
const workflowWithSentinel = `${workflow}\n  __end__:\n`;

function jobBody(name) {
  const match = workflowWithSentinel.match(
    new RegExp(`^  ${name}:\\n([\\s\\S]*?)(?=^  [a-zA-Z0-9_]+:\\n)`, "m"),
  );
  assert.ok(match, `missing ${name} job`);
  return match[1];
}

function stepRunScript(job, stepName) {
  const stepMarker = `      - name: ${stepName}\n`;
  const stepStart = job.indexOf(stepMarker);
  assert.notEqual(stepStart, -1, `missing ${stepName} step`);

  const afterStep = job.slice(stepStart + stepMarker.length);
  const nextStep = afterStep.indexOf("\n      - name:");
  const step = nextStep === -1 ? afterStep : afterStep.slice(0, nextStep);
  const runMarker = "        run: |\n";
  const runStart = step.indexOf(runMarker);
  assert.notEqual(runStart, -1, `missing run script for ${stepName}`);

  return step
    .slice(runStart + runMarker.length)
    .split("\n")
    .map((line) => (line === "" ? line : line.slice(10)))
    .join("\n");
}

const fakeDocker = String.raw`#!/usr/bin/env node
import { readFileSync, writeFileSync } from "node:fs";

const statePath = process.env.FAKE_DOCKER_STATE;
const state = JSON.parse(readFileSync(statePath, "utf8"));
const [group, tool, command, ...args] = process.argv.slice(2);

function save() {
  writeFileSync(statePath, JSON.stringify(state));
}

if (group !== "buildx" || tool !== "imagetools") {
  console.error("unsupported fake docker command");
  process.exit(2);
}

if (command === "inspect") {
  const ref = args[0];
  if ((state.failInspect?.[ref] ?? 0) > 0) {
    state.failInspect[ref] -= 1;
    save();
    console.error("injected inspect failure for " + ref);
    process.exit(1);
  }
  const value = state.refs[ref];
  if (!value) {
    console.error("manifest unknown: " + ref);
    process.exit(1);
  }
  if (args.includes("--format")) {
    const format = args[args.indexOf("--format") + 1] ?? "";
    const match = format.match(
      /^\{\{index \(index \.Image "(linux\/(?:amd64|arm64))"\)\.Config\.Labels "org\.opencontainers\.image\.(version|revision)"\}\}$/,
    );
    if (!match) {
      console.error("unsupported fake inspect format: " + format);
      process.exit(2);
    }
    const platformValues = match[2] === "version" ? value.platformVersions : value.platformRevisions;
    console.log(platformValues?.[match[1]] ?? value[match[2]] ?? "");
  } else {
    console.log("Name: " + ref);
    console.log("Digest: " + value.digest);
  }
  process.exit(0);
}

if (command === "create") {
  const targetIndex = args.indexOf("-t");
  const target = targetIndex >= 0 ? args[targetIndex + 1] : "";
  const sources = args.filter((_, index) => index !== targetIndex && index !== targetIndex + 1);
  if ((state.failCreate?.[target] ?? 0) > 0) {
    state.failCreate[target] -= 1;
    save();
    console.error("injected create failure for " + target);
    process.exit(1);
  }
  const values = sources.map((source) => {
    let value = state.refs[source];
    const digestSeparator = source.lastIndexOf("@sha256:");
    if (!value && digestSeparator >= 0) {
      const digest = source.slice(digestSeparator + 1);
      value = Object.values(state.refs).find((candidate) => candidate.digest === digest) ?? {
        digest,
        version: "",
      };
    }
    return value;
  });
  if (!target || values.length === 0 || values.some((value) => !value)) {
    console.error("unknown fake create source or target");
    process.exit(2);
  }
  let value;
  if (values.length === 1) {
    value = { ...values[0] };
  } else {
    const platformVersions = {};
    const platformRevisions = {};
    for (let index = 0; index < sources.length; index += 1) {
      Object.assign(platformVersions, values[index].platformVersions ?? {});
      Object.assign(platformRevisions, values[index].platformRevisions ?? {});
      if (sources[index].endsWith("-amd64")) {
        platformVersions["linux/amd64"] = values[index].version ?? "";
        platformRevisions["linux/amd64"] = values[index].revision ?? "";
      }
      if (sources[index].endsWith("-arm64")) {
        platformVersions["linux/arm64"] = values[index].version ?? "";
        platformRevisions["linux/arm64"] = values[index].revision ?? "";
      }
    }
    value = {
      digest: values[0].digest,
      platformVersions,
      platformRevisions,
    };
  }
  state.refs[target] = value;
  save();
  process.exit(0);
}

console.error("unsupported fake imagetools command");
process.exit(2);
`;

function runWithFakeDocker(script, initialState, environment = {}, options = {}) {
  const directory = mkdtempSync(join(tmpdir(), "sub2api-release-test-"));
  const dockerPath = join(directory, "docker");
  const statePath = join(directory, "state.json");
  const outputPath = join(directory, "github-output");
  try {
    writeFileSync(dockerPath, fakeDocker);
    chmodSync(dockerPath, 0o755);
    writeFileSync(statePath, JSON.stringify(initialState));
    writeFileSync(outputPath, "");
    const result = spawnSync("bash", ["-c", script], {
      encoding: "utf8",
      env: {
        ...process.env,
        ...environment,
        PATH: `${directory}:${process.env.PATH}`,
        FAKE_DOCKER_STATE: statePath,
        GITHUB_OUTPUT: options.failOutput
          ? join(directory, "missing", "github-output")
          : outputPath,
        RUNNER_TEMP: directory,
      },
    });
    return {
      result,
      state: JSON.parse(readFileSync(statePath, "utf8")),
      output: result.status === 0 ? readFileSync(outputPath, "utf8") : "",
    };
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

function digest(character) {
  return `sha256:${character.repeat(64)}`;
}

const apiImage = "registry.example/sub2api-console-api";
const frontendImage = "registry.example/sub2api-console-frontend";
const previousTag = "v2026.08.31";
const releaseTag = "v2026.09.01";
const previousRevision = "previous-revision";
const releaseRevision = "release-revision";

function promotionState() {
  return {
    refs: {
      [`${apiImage}:latest`]: {
        digest: digest("a"),
        version: previousTag,
        revision: previousRevision,
      },
      [`${frontendImage}:latest`]: {
        digest: digest("b"),
        version: previousTag,
        revision: previousRevision,
      },
      [`${apiImage}:${releaseTag}`]: {
        digest: digest("c"),
        version: releaseTag,
        revision: releaseRevision,
      },
      [`${frontendImage}:${releaseTag}`]: {
        digest: digest("d"),
        version: releaseTag,
        revision: releaseRevision,
      },
    },
    failCreate: {},
    failInspect: {},
  };
}

function promotionEnvironment() {
  return {
    API_IMAGE: apiImage,
    FRONTEND_IMAGE: frontendImage,
    RELEASE_TAG: releaseTag,
    GITHUB_SHA: releaseRevision,
  };
}

test("architecture builds publish versioned tags without moving latest tags", () => {
  const images = jobBody("images");

  assert.match(images, /\$\{\{ matrix\.image }}:\$\{\{ github\.ref_name }}-\$\{\{ matrix\.arch }}/);
  assert.doesNotMatch(images, /latest-/);
  assert.doesNotMatch(workflow, /:latest-(?:amd64|arm64|\$\{\{)/);
});

test("version manifests are built and verified before latest promotion", () => {
  const manifests = jobBody("manifests");
  const promotion = jobBody("promote_latest");

  assert.match(manifests, /needs: images/);
  assert.match(manifests, /\$\{GITHUB_REF_NAME}-amd64/);
  assert.match(manifests, /\$\{GITHUB_REF_NAME}-arm64/);
  assert.match(manifests, /docker buildx imagetools create/);
  assert.match(manifests, /docker buildx imagetools inspect/);
  assert.match(manifests, /org\.opencontainers\.image\.version/);
  assert.match(manifests, /org\.opencontainers\.image\.revision/);
  assert.match(manifests, /revision.*GITHUB_SHA/);
  assert.doesNotMatch(manifests, /:latest/);
  assert.match(promotion, /needs: manifests/);
  assert.doesNotMatch(promotion, /matrix:/);
});

test("registry order is guarded before release preflight and image publication", () => {
  const guard = jobBody("registry_guard");
  const preflight = jobBody("preflight");
  const images = jobBody("images");

  assert.match(guard, /needs: validate/);
  assert.match(guard, /node \.github\/scripts\/validate-release-order\.mjs/);
  assert.match(preflight, /needs: registry_guard/);
  assert.match(images, /needs: preflight/);
});

test("version manifest script verifies each platform version and revision", () => {
  const script = stepRunScript(jobBody("manifests"), "Publish version manifest").replaceAll(
    "${{ matrix.image }}",
    apiImage,
  );
  const state = {
    refs: {
      [`${apiImage}:${releaseTag}-amd64`]: {
        digest: digest("a"),
        version: releaseTag,
        revision: releaseRevision,
      },
      [`${apiImage}:${releaseTag}-arm64`]: {
        digest: digest("b"),
        version: releaseTag,
        revision: releaseRevision,
      },
    },
    failCreate: {},
    failInspect: {},
  };
  const success = runWithFakeDocker(script, state, {
    GITHUB_REF_NAME: releaseTag,
    GITHUB_SHA: releaseRevision,
  });

  assert.equal(success.result.status, 0, success.result.stderr);
  assert.deepEqual(success.state.refs[`${apiImage}:${releaseTag}`].platformVersions, {
    "linux/amd64": releaseTag,
    "linux/arm64": releaseTag,
  });
  assert.deepEqual(success.state.refs[`${apiImage}:${releaseTag}`].platformRevisions, {
    "linux/amd64": releaseRevision,
    "linux/arm64": releaseRevision,
  });

  state.refs[`${apiImage}:${releaseTag}-arm64`].version = previousTag;
  const versionMismatch = runWithFakeDocker(script, state, {
    GITHUB_REF_NAME: releaseTag,
    GITHUB_SHA: releaseRevision,
  });
  assert.notEqual(versionMismatch.result.status, 0);
  assert.match(versionMismatch.result.stderr, /linux\/arm64 reports version/);

  state.refs[`${apiImage}:${releaseTag}-arm64`].version = releaseTag;
  state.refs[`${apiImage}:${releaseTag}-arm64`].revision = previousRevision;
  const revisionMismatch = runWithFakeDocker(script, state, {
    GITHUB_REF_NAME: releaseTag,
    GITHUB_SHA: releaseRevision,
  });
  assert.notEqual(revisionMismatch.result.status, 0);
  assert.match(revisionMismatch.result.stderr, /linux\/arm64 reports revision/);
});

test("one promotion step advances both latest tags and rolls back failures", () => {
  const promotion = jobBody("promote_latest");

  assert.match(promotion, /name: Promote both service manifests/);
  assert.match(promotion, /API_IMAGE: ghcr\.io\/mienchating\/sub2api-console-api/);
  assert.match(promotion, /FRONTEND_IMAGE: ghcr\.io\/mienchating\/sub2api-console-frontend/);
  assert.match(promotion, /api_previous_digest="\$\(snapshot_latest/);
  assert.match(promotion, /frontend_previous_digest="\$\(snapshot_latest/);
  assert.match(promotion, /trap rollback_promotions EXIT/);
  assert.match(promotion, /trap - EXIT/);
  assert.match(promotion, /restore_latest "\$API_IMAGE" "\$api_previous_digest"/);
  assert.match(promotion, /restore_latest "\$FRONTEND_IMAGE" "\$frontend_previous_digest"/);
  assert.equal((promotion.match(/-t "\$\{(?:API|FRONTEND)_IMAGE}:latest"/g) ?? []).length, 2);
  assert.match(promotion, /verify_digest "\$\{API_IMAGE}:latest" "\$api_release_digest"/);
  assert.match(promotion, /verify_digest "\$\{FRONTEND_IMAGE}:latest" "\$frontend_release_digest"/);
  assert.match(promotion, /api_previous_version="\$\(label_of/);
  assert.match(promotion, /frontend_previous_version="\$\(label_of/);
  assert.match(promotion, /Refusing to promote a mismatched latest pair/);
  assert.match(promotion, /api_previous_revision="\$\(label_of/);
  assert.match(promotion, /frontend_previous_revision="\$\(label_of/);
  assert.match(promotion, /Refusing to promote latest tags from different revisions/);
  assert.match(promotion, /api_release_revision=/);
  assert.match(promotion, /frontend_release_revision=/);
  assert.match(promotion, /\$api_release_revision.*\$GITHUB_SHA/);
  assert.match(promotion, /\$frontend_release_revision.*\$GITHUB_SHA/);
  assert.match(promotion, /api_previous_digest=\$api_previous_digest/);
  assert.match(promotion, /frontend_previous_digest=\$frontend_previous_digest/);
});

test("latest promotion rejects absent or partial pairs before mutating either tag", () => {
  const script = stepRunScript(jobBody("promote_latest"), "Promote both service manifests");
  const partialPairGuard = script.indexOf(
    "Refusing to promote from a partially initialized latest pair",
  );
  const absentPairGuard = script.indexOf("Refusing to bootstrap latest tags automatically");
  const firstMutation = script.indexOf("api_mutation_started=true");

  assert.notEqual(partialPairGuard, -1);
  assert.notEqual(absentPairGuard, -1);
  assert.notEqual(firstMutation, -1);
  assert.ok(partialPairGuard < firstMutation);
  assert.ok(absentPairGuard < firstMutation);
  assert.doesNotMatch(script, /Cannot restore absent/);
});

test("release mutation shells are syntactically valid Bash", () => {
  const scripts = [
    stepRunScript(jobBody("promote_latest"), "Promote both service manifests"),
    stepRunScript(jobBody("rollback_latest"), "Restore previous service manifests"),
  ];
  for (const script of scripts) {
    const result = spawnSync("bash", ["-n"], { encoding: "utf8", input: script });
    assert.equal(result.status, 0, result.stderr);
  }
});

test("promotion script advances a verified pair and exports rollback digests", () => {
  const script = stepRunScript(jobBody("promote_latest"), "Promote both service manifests");
  const run = runWithFakeDocker(script, promotionState(), promotionEnvironment());

  assert.equal(run.result.status, 0, run.result.stderr);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("c"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("d"));
  assert.match(run.output, new RegExp(`api_previous_digest=${digest("a")}`));
  assert.match(run.output, new RegExp(`frontend_previous_digest=${digest("b")}`));
});

test("promotion script rejects a mismatched previous pair before mutation", () => {
  const script = stepRunScript(jobBody("promote_latest"), "Promote both service manifests");
  const state = promotionState();
  state.refs[`${frontendImage}:latest`].version = "v2026.08.30";
  const run = runWithFakeDocker(script, state, promotionEnvironment());

  assert.notEqual(run.result.status, 0);
  assert.match(run.result.stderr, /Refusing to promote a mismatched latest pair/);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("promotion script rejects previous latest tags from different revisions", () => {
  const script = stepRunScript(jobBody("promote_latest"), "Promote both service manifests");
  const state = promotionState();
  state.refs[`${frontendImage}:latest`].revision = "different-previous-revision";
  const run = runWithFakeDocker(script, state, promotionEnvironment());

  assert.notEqual(run.result.status, 0);
  assert.match(run.result.stderr, /latest tags from different revisions/);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("promotion script rejects an older release before mutating latest", () => {
  const script = stepRunScript(jobBody("promote_latest"), "Promote both service manifests");
  const state = promotionState();
  state.refs[`${apiImage}:latest`].version = "v2026.09.02";
  state.refs[`${frontendImage}:latest`].version = "v2026.09.02";
  const run = runWithFakeDocker(script, state, promotionEnvironment());

  assert.notEqual(run.result.status, 0);
  assert.match(run.result.stderr, /older than current latest/);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("registry guard rejects an older release before image jobs can run", () => {
  const script = stepRunScript(jobBody("registry_guard"), "Validate current latest pair");
  const state = promotionState();
  state.refs[`${apiImage}:latest`].version = "v2026.09.02";
  state.refs[`${frontendImage}:latest`].version = "v2026.09.02";
  const run = runWithFakeDocker(script, state, promotionEnvironment());

  assert.notEqual(run.result.status, 0);
  assert.match(run.result.stderr, /older than current latest/);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("registry guard allows an absent pair but rejects a partial pair", () => {
  const script = stepRunScript(jobBody("registry_guard"), "Validate current latest pair");
  const absent = promotionState();
  delete absent.refs[`${apiImage}:latest`];
  delete absent.refs[`${frontendImage}:latest`];
  const absentRun = runWithFakeDocker(script, absent, promotionEnvironment());

  assert.equal(absentRun.result.status, 0, absentRun.result.stderr);
  assert.match(absentRun.result.stdout, /No latest pair exists/);

  const partial = promotionState();
  delete partial.refs[`${frontendImage}:latest`];
  const partialRun = runWithFakeDocker(script, partial, promotionEnvironment());

  assert.notEqual(partialRun.result.status, 0);
  assert.match(partialRun.result.stderr, /partially initialized latest pair/);
});

test("registry guard rejects an existing release tag from another revision", () => {
  const script = stepRunScript(jobBody("registry_guard"), "Validate current latest pair");
  const state = promotionState();
  state.refs[`${frontendImage}:${releaseTag}`].revision = "different-release-revision";
  const run = runWithFakeDocker(script, state, promotionEnvironment());

  assert.notEqual(run.result.status, 0);
  assert.match(run.result.stderr, /Refusing to overwrite/);
  assert.equal(run.state.refs[`${frontendImage}:${releaseTag}`].revision, "different-release-revision");
});

test("promotion script rejects reusing a release tag for another revision", () => {
  const script = stepRunScript(jobBody("promote_latest"), "Promote both service manifests");
  const state = promotionState();
  state.refs[`${apiImage}:latest`].version = releaseTag;
  state.refs[`${frontendImage}:latest`].version = releaseTag;
  const run = runWithFakeDocker(script, state, promotionEnvironment());

  assert.notEqual(run.result.status, 0);
  assert.match(run.result.stderr, /Refusing to reuse/);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("promotion script rejects a previous manifest with mixed architecture revisions", () => {
  const script = stepRunScript(jobBody("promote_latest"), "Promote both service manifests");
  const state = promotionState();
  state.refs[`${apiImage}:latest`].platformRevisions = {
    "linux/amd64": previousRevision,
    "linux/arm64": "different-previous-revision",
  };
  const run = runWithFakeDocker(script, state, promotionEnvironment());

  assert.notEqual(run.result.status, 0);
  assert.match(run.result.stderr, /consistent revision label across amd64 and arm64/);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("promotion script rejects a release manifest from another revision", () => {
  const script = stepRunScript(jobBody("promote_latest"), "Promote both service manifests");
  const state = promotionState();
  state.refs[`${frontendImage}:${releaseTag}`].revision = "different-release-revision";
  const run = runWithFakeDocker(script, state, promotionEnvironment());

  assert.notEqual(run.result.status, 0);
  assert.match(run.result.stderr, /do not both report release revision/);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("promotion script restores both tags when the second mutation fails", () => {
  const script = stepRunScript(jobBody("promote_latest"), "Promote both service manifests");
  const state = promotionState();
  state.failCreate[`${frontendImage}:latest`] = 1;
  const run = runWithFakeDocker(script, state, promotionEnvironment());

  assert.notEqual(run.result.status, 0);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("promotion script restores both tags when exporting job outputs fails", () => {
  const script = stepRunScript(jobBody("promote_latest"), "Promote both service manifests");
  const run = runWithFakeDocker(script, promotionState(), promotionEnvironment(), {
    failOutput: true,
  });

  assert.notEqual(run.result.status, 0);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("release validation runs all workflow tests and publication waits for promotion", () => {
  assert.match(jobBody("validate"), /node --test \.github\/scripts\/\*\.test\.mjs/);
  assert.match(jobBody("release"), /needs: promote_latest/);
  assert.doesNotMatch(workflow, /IMAGE_PREFIX/);
});

test("release failure restores both previous latest digests", () => {
  const rollback = jobBody("rollback_latest");

  assert.match(
    rollback,
    /if: \$\{\{ always\(\) && needs\.promote_latest\.result == 'success' && needs\.release\.result != 'success' }}/,
  );
  assert.match(rollback, /needs\.promote_latest\.outputs\.api_previous_digest/);
  assert.match(rollback, /needs\.promote_latest\.outputs\.frontend_previous_digest/);
  assert.match(rollback, /restore_latest "\$FRONTEND_IMAGE" "\$FRONTEND_PREVIOUS_DIGEST"/);
  assert.match(rollback, /restore_latest "\$API_IMAGE" "\$API_PREVIOUS_DIGEST"/);
});

test("release compensation script restores the previously exported pair", () => {
  const script = stepRunScript(jobBody("rollback_latest"), "Restore previous service manifests");
  const state = promotionState();
  state.refs[`${apiImage}:old`] = { digest: digest("a"), version: previousTag };
  state.refs[`${frontendImage}:old`] = { digest: digest("b"), version: previousTag };
  state.refs[`${apiImage}:latest`] = { digest: digest("c"), version: releaseTag };
  state.refs[`${frontendImage}:latest`] = { digest: digest("d"), version: releaseTag };
  const run = runWithFakeDocker(script, state, {
    API_IMAGE: apiImage,
    FRONTEND_IMAGE: frontendImage,
    API_PREVIOUS_DIGEST: digest("a"),
    FRONTEND_PREVIOUS_DIGEST: digest("b"),
  });

  assert.equal(run.result.status, 0, run.result.stderr);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("release compensation retries a transient digest inspection failure", () => {
  const script = stepRunScript(jobBody("rollback_latest"), "Restore previous service manifests");
  const state = promotionState();
  state.refs[`${apiImage}:latest`] = { digest: digest("c"), version: releaseTag };
  state.refs[`${frontendImage}:latest`] = { digest: digest("d"), version: releaseTag };
  state.failInspect[`${frontendImage}:latest`] = 1;
  const run = runWithFakeDocker(script, state, {
    API_IMAGE: apiImage,
    FRONTEND_IMAGE: frontendImage,
    API_PREVIOUS_DIGEST: digest("a"),
    FRONTEND_PREVIOUS_DIGEST: digest("b"),
  });

  assert.equal(run.result.status, 0, run.result.stderr);
  assert.equal(run.state.refs[`${apiImage}:latest`].digest, digest("a"));
  assert.equal(run.state.refs[`${frontendImage}:latest`].digest, digest("b"));
});

test("jobs use least-privilege token scopes", () => {
  assert.match(workflow, /^permissions: \{}/m);
  assert.doesNotMatch(workflow, /id-token:/);
  assert.match(jobBody("validate"), /permissions:\n      contents: read/);
  assert.match(
    jobBody("registry_guard"),
    /permissions:\n      contents: read\n      packages: read/,
  );
  assert.match(jobBody("preflight"), /permissions:\n      contents: read/);
  assert.match(jobBody("images"), /permissions:\n      contents: read\n      packages: write/);
  assert.match(jobBody("manifests"), /permissions:\n      packages: write/);
  assert.match(
    jobBody("promote_latest"),
    /permissions:\n      contents: read\n      packages: write/,
  );
  assert.match(jobBody("release"), /permissions:\n      contents: write/);
  assert.match(jobBody("rollback_latest"), /permissions:\n      packages: write/);
});
