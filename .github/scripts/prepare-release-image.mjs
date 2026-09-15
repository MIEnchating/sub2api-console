import { appendFileSync } from "node:fs";

import { readReleaseImage, releaseEnvironment, requireCandidate, requirePromotion } from "./release-images.mjs";

const release = releaseEnvironment();
const current = readReleaseImage(release.image, "latest", true);
if (current) requirePromotion(current, release.version, release.revision);
const candidate = readReleaseImage(release.image, release.version, true);
if (candidate) requireCandidate(candidate, release.version, release.revision);
if (!process.env.GITHUB_OUTPUT) throw new Error("GITHUB_OUTPUT is required");
appendFileSync(process.env.GITHUB_OUTPUT, `reuse=${candidate !== null}\n`);
