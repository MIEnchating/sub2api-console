import assert from "node:assert/strict";
import test from "node:test";

import { releaseImageMetadata } from "./release-images.mjs";

function platform(version = "v2026.09.14", revision = "commit-a") {
  return { config: { Labels: {
    "org.opencontainers.image.version": version,
    "org.opencontainers.image.revision": revision,
  } } };
}

test("matching amd64 and arm64 release labels produce one immutable release identity", () => {
  assert.deepEqual(releaseImageMetadata({ "linux/amd64": platform(), "linux/arm64": platform() }), {
    version: "v2026.09.14", revision: "commit-a",
  });
});

test("missing architecture or metadata cannot be promoted as a complete release", () => {
  for (const image of [null, {}, { "linux/amd64": platform() }, {
    "linux/amd64": platform(), "linux/arm64": { config: { Labels: {} } },
  }]) assert.throws(() => releaseImageMetadata(image));
});

test("mixed architecture versions or revisions are rejected before latest is updated", () => {
  for (const arm of [platform("v2026.09.13"), platform("v2026.09.14", "commit-b")]) {
    assert.throws(() => releaseImageMetadata({ "linux/amd64": platform(), "linux/arm64": arm }));
  }
});

test("invalid release version cannot bypass the calendar ordering check", () => {
  assert.throws(() => releaseImageMetadata({
    "linux/amd64": platform("v2026.02.30"), "linux/arm64": platform("v2026.02.30"),
  }));
});
