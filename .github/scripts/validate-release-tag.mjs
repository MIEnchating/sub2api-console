import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

const releaseTagPattern = /^v(\d{4})\.(\d{2})\.(\d{2})(?:-([2-9]|[1-9]\d+))?$/;
const requiredReleaseSections = ["版本概览", "新增功能", "功能改进", "问题修复", "移除与调整"];

function requireValidCalendarDate(year, month, day, tag) {
  const parsed = new Date(Date.UTC(year, month - 1, day));
  if (
    parsed.getUTCFullYear() !== year ||
    parsed.getUTCMonth() !== month - 1 ||
    parsed.getUTCDate() !== day
  ) {
    throw new Error(`Release tag contains an invalid calendar date: ${tag}`);
  }
}

function requirePreviousTag(baseTag, revision) {
  if (revision === undefined) return;
  const previousTag = revision === 2 ? baseTag : `${baseTag}-${revision - 1}`;
  try {
    execFileSync("git", ["show-ref", "--verify", "--quiet", `refs/tags/${previousTag}`]);
  } catch {
    throw new Error(`Release tag ${baseTag}-${revision} requires previous tag ${previousTag}`);
  }
}

export function validateReleaseNotesContent(content, sourcePath = "release notes") {
  const headingMatches = [...content.matchAll(/^##\s+(.+?)\s*$/gm)];
  const headings = headingMatches.map((match) => match[1]);
  if (
    headings.length !== requiredReleaseSections.length ||
    headings.some((heading, index) => heading !== requiredReleaseSections[index])
  ) {
    throw new Error(
      `${sourcePath} must contain exactly these sections in order: ${requiredReleaseSections.join(", ")}`,
    );
  }
  if (/<!--\s*请|TODO|待补充/i.test(content)) {
    throw new Error(`${sourcePath} still contains unfinished placeholders`);
  }
  const emptySections = headingMatches
    .filter((match, index) => {
      const bodyStart = match.index + match[0].length;
      const bodyEnd = headingMatches[index + 1]?.index ?? content.length;
      return content.slice(bodyStart, bodyEnd).replace(/<!--[\s\S]*?-->/g, "").trim().length === 0;
    })
    .map((match) => match[1]);
  if (emptySections.length > 0) {
    throw new Error(
      `${sourcePath} has empty required sections: ${emptySections.join(", ")}. Use - 无 when a section has no entries.`,
    );
  }
}

export function validateReleaseTag(tag, options = {}) {
  const match = releaseTagPattern.exec(tag);
  if (!match) {
    throw new Error(
      `Unsupported release tag: ${tag}. Use vYYYY.MM.DD for the first release, then vYYYY.MM.DD-2, -3, and so on. The -1 suffix is forbidden.`,
    );
  }
  const [, yearText, monthText, dayText, revisionText] = match;
  requireValidCalendarDate(Number(yearText), Number(monthText), Number(dayText), tag);
  const baseTag = `v${yearText}.${monthText}.${dayText}`;
  const revision = revisionText === undefined ? undefined : Number(revisionText);
  if (options.checkSequence !== false) requirePreviousTag(baseTag, revision);
  return { baseTag, revision };
}

function requireReleaseNotes(tag) {
  const sourcePath = `.github/release-notes/${tag}.md`;
  if (!existsSync(sourcePath)) {
    throw new Error(`Missing ${sourcePath}. Complete and commit the release notes before tagging.`);
  }
  validateReleaseNotesContent(readFileSync(sourcePath, "utf8"), sourcePath);
}

const entryPath = process.argv[1] ? pathToFileURL(process.argv[1]).href : "";
if (import.meta.url === entryPath) {
  const tag = process.argv[2];
  if (!tag) throw new Error("Release tag is required");
  validateReleaseTag(tag);
  requireReleaseNotes(tag);
  console.log(`Validated release tag and notes: ${tag}`);
}
