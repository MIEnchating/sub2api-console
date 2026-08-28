import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

import { validateReleaseNotesContent, validateReleaseTag } from "./validate-release-tag.mjs";

const [tag = process.env.GITHUB_REF_NAME, outputPath = "release-notes.md"] = process.argv.slice(2);
if (!tag) throw new Error("Release tag is required");
validateReleaseTag(tag);

const sourcePath = join(".github", "release-notes", `${tag}.md`);
if (!existsSync(sourcePath)) {
  throw new Error(
    `Missing ${sourcePath}. Copy TEMPLATE.md, complete every section, commit it, and create the tag again.`,
  );
}
const content = readFileSync(sourcePath, "utf8").trim();
validateReleaseNotesContent(content, sourcePath);
writeFileSync(outputPath, `${content}\n`);
