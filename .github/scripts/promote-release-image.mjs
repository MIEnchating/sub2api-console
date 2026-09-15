import { execFileSync } from "node:child_process";

import { readReleaseImage, releaseEnvironment, requireCandidate, requirePromotion } from "./release-images.mjs";

const release = releaseEnvironment();
const candidate = readReleaseImage(release.image, release.version);
requireCandidate(candidate, release.version, release.revision);
const current = readReleaseImage(release.image, "latest", true);
if (!current) {
  throw new Error(`Version ${release.version} is verified. Initialize latest from this version digest before rerunning; see .github/release-notes/README.md`);
}
requirePromotion(current, release.version, release.revision);
if (current.digest !== candidate.digest) {
  execFileSync("docker", ["buildx", "imagetools", "create", "--tag", `${release.image}:latest`, `${release.image}@${candidate.digest}`], {
    stdio: "inherit", timeout: 120_000,
  });
}
const promoted = readReleaseImage(release.image, "latest");
if (promoted.digest !== candidate.digest) throw new Error("latest digest verification failed");
