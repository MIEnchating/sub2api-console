import { execFileSync } from "node:child_process";

import { validateReleaseOrder, validateReleaseTag } from "./validate-release-tag.mjs";

export function releaseImageMetadata(images) {
  let identity;
  for (const platform of ["linux/amd64", "linux/arm64"]) {
    const labels = images?.[platform]?.config?.Labels;
    const version = labels?.["org.opencontainers.image.version"];
    const revision = labels?.["org.opencontainers.image.revision"];
    if (typeof version !== "string" || typeof revision !== "string" || !revision.trim()) {
      throw new Error(`Missing release labels for ${platform}`);
    }
    validateReleaseTag(version, { checkSequence: false });
    if (identity && (identity.version !== version || identity.revision !== revision)) {
      throw new Error("Release architectures contain different versions or revisions");
    }
    identity = { version, revision };
  }
  return identity;
}

function inspect(reference, format) {
  return execFileSync("docker", ["buildx", "imagetools", "inspect", reference, "--format", format], {
    encoding: "utf8", timeout: 120_000, stdio: ["ignore", "pipe", "pipe"],
  });
}

export function readReleaseImage(image, tag, allowMissing = false) {
  let manifest;
  try {
    manifest = JSON.parse(inspect(`${image}:${tag}`, "{{json .Manifest}}"));
  } catch (error) {
    // Authentication, rate limits and transport errors must never mean "absent".
    if (allowMissing && /manifest unknown|no such manifest|: not found(?:\s|$)/i.test(String(error.stderr ?? ""))) {
      return null;
    }
    throw error;
  }
  if (!/^sha256:[a-f0-9]{64}$/.test(manifest.digest ?? "")) {
    throw new Error(`Invalid manifest digest for ${image}:${tag}`);
  }
  const images = JSON.parse(inspect(`${image}@${manifest.digest}`, "{{json .Image}}"));
  return { ...releaseImageMetadata(images), digest: manifest.digest };
}

export function requireCandidate(candidate, version, revision) {
  if (candidate.version !== version || candidate.revision !== revision) {
    throw new Error(`Published ${version} does not match the requested version and revision`);
  }
}

export function requirePromotion(current, version, revision) {
  validateReleaseOrder(current.version, version, current.revision, revision);
}

export function releaseEnvironment() {
  const image = process.env.IMAGE;
  const version = process.env.GITHUB_REF_NAME;
  const revision = process.env.GITHUB_SHA;
  if (!image || !version || !revision) throw new Error("IMAGE, GITHUB_REF_NAME and GITHUB_SHA are required");
  validateReleaseTag(version, { checkSequence: false });
  return { image, version, revision };
}
